// Package observability exposes bounded, read-only, loopback-only operational data.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
)

type Queue struct {
	Pending              int       `json:"pending"`
	Outbox               int       `json:"outbox"`
	Receipts             int       `json:"receipts"`
	OldestPendingSeconds float64   `json:"oldest_pending_seconds"`
	OldestOutboxSeconds  float64   `json:"oldest_outbox_seconds"`
	LastCompleted        time.Time `json:"last_completed,omitempty"`
}

type Snapshot struct {
	Online          bool                       `json:"online"`
	Timestamp       time.Time                  `json:"timestamp"`
	StartedAt       time.Time                  `json:"started_at"`
	Version         string                     `json:"version"`
	BrokerConnected bool                       `json:"broker_connected"`
	DockerReady     bool                       `json:"docker_ready"`
	ObservedAt      time.Time                  `json:"observed_at"`
	Queue           Queue                      `json:"queue"`
	Deployments     []control.DeploymentStatus `json:"deployments"`
}

type State struct {
	mu          sync.RWMutex
	version     string
	started     time.Time
	observed    time.Time
	dockerReady bool
	deployments []control.DeploymentStatus
	queue       func() Queue
	connected   func() bool
}

func New(version string, queue func() Queue, connected func() bool) *State {
	return &State{version: version, started: time.Now().UTC(), queue: queue, connected: connected}
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	v := Snapshot{Online: true, Timestamp: time.Now().UTC(), StartedAt: s.started, Version: s.version,
		ObservedAt: s.observed, DockerReady: s.dockerReady, Deployments: append([]control.DeploymentStatus(nil), s.deployments...)}
	s.mu.RUnlock()
	if s.queue != nil {
		v.Queue = s.queue()
	}
	if s.connected != nil {
		v.BrokerConnected = s.connected()
	}
	return v
}

// Observe runs one bounded observation at a time. A failed observation clears
// readiness but retains the timestamp of the last successful observation.
func (s *State) Observe(ctx context.Context, runtime control.Runtime, interval time.Duration) {
	observe := func() {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		result, err := runtime.Execute(probeCtx, "status", control.Request{})
		deployments, ok := result.([]control.DeploymentStatus)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.dockerReady = err == nil && ok
		if s.dockerReady {
			s.deployments = deployments
			s.observed = time.Now().UTC()
		}
	}
	observe()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			observe()
		}
	}
}

func ValidateListen(address string) error {
	if address == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("monitor_listen must be a numeric loopback address and port")
	}
	ip := net.ParseIP(host)
	n, err := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("monitor_listen must use loopback (127.0.0.1 or ::1); use an authenticated proxy for remote access")
	}
	return nil
}

func (s *State) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		v := s.Snapshot()
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "ok")
		case "/readyz":
			if !v.BrokerConnected || !v.DockerReady || v.ObservedAt.IsZero() || time.Since(v.ObservedAt) > time.Minute {
				http.Error(w, "dependencies unavailable or stale", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprintln(w, "ready")
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			gauge := func(name, help string, value any) {
				fmt.Fprintf(w, "# HELP captain_compose_%s %s\n# TYPE captain_compose_%s gauge\ncaptain_compose_%s %v\n", name, help, name, name, value)
			}
			boolean := func(value bool) int {
				if value {
					return 1
				}
				return 0
			}
			gauge("broker_connected", "Whether the agent is connected to MQTT.", boolean(v.BrokerConnected))
			gauge("docker_ready", "Whether the last Docker observation succeeded.", boolean(v.DockerReady))
			gauge("uptime_seconds", "Agent process uptime in seconds.", time.Since(v.StartedAt).Seconds())
			gauge("pending_commands", "Commands awaiting durable acceptance or execution.", v.Queue.Pending)
			gauge("pending_events", "Results waiting for broker acknowledgement.", v.Queue.Outbox)
			gauge("retained_receipts", "Retained command receipts.", v.Queue.Receipts)
			gauge("oldest_pending_seconds", "Age of the oldest unexecuted command.", v.Queue.OldestPendingSeconds)
			gauge("oldest_event_seconds", "Age of the oldest undelivered result.", v.Queue.OldestOutboxSeconds)
			gauge("managed_deployments", "Number of managed deployment intents observed.", len(v.Deployments))
			var drift, unhealthy int
			for _, d := range v.Deployments {
				if d.Drift {
					drift++
				}
				for _, svc := range d.Services {
					if svc.Health == "unhealthy" || svc.State == "exited" || svc.State == "dead" {
						unhealthy++
					}
				}
			}
			gauge("drifted_deployments", "Managed deployments with observed drift.", drift)
			gauge("unhealthy_services", "Observed unhealthy or stopped containers.", unhealthy)
			age := -1.0
			if !v.ObservedAt.IsZero() {
				age = time.Since(v.ObservedAt).Seconds()
			}
			gauge("observation_age_seconds", "Age of last successful observation; -1 if never successful.", age)
		default:
			http.NotFound(w, r)
		}
	})
}

// Serve starts a loopback-only HTTP server. Cancellation closes listeners and
// active connections; operational requests cannot execute deployment actions.
func (s *State) Serve(ctx context.Context, address string) (net.Addr, <-chan error, error) {
	if err := ValidateListen(address); err != nil {
		return nil, nil, err
	}
	if address == "" {
		return nil, nil, fmt.Errorf("monitor address is empty")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, err
	}
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		done <- err
		close(done)
	}()
	go func() { <-ctx.Done(); _ = server.Close() }()
	return listener.Addr(), done, nil
}
