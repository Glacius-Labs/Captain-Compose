package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
)

func TestMonitoringDoesNotConfuseLivenessWithReadiness(t *testing.T) {
	s := New("test", func() Queue { return Queue{Pending: 2, Outbox: 3} }, func() bool { return true })
	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/status"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		want := http.StatusOK
		if path == "/readyz" {
			want = http.StatusServiceUnavailable
		}
		if w.Code != want {
			t.Fatalf("%s: status %d", path, w.Code)
		}
		if path == "/metrics" && !strings.Contains(w.Body.String(), "captain_compose_pending_events 3") {
			t.Fatal(w.Body.String())
		}
	}
	s.dockerReady = true
	s.observed = time.Now()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	s.observed = time.Now().Add(-2 * time.Minute)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("stale Docker observation was ready")
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/status", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal("monitor accepted a mutation method")
	}
}

type observationRuntime struct{ calls atomic.Int32 }

func (r *observationRuntime) Execute(ctx context.Context, action string, request control.Request) (any, error) {
	if r.calls.Add(1) == 1 {
		return []control.DeploymentStatus{{Name: "web", Phase: "active"}}, nil
	}
	return nil, errors.New("Docker unavailable")
}

func TestObservationFailureClearsReadinessAndPreservesTimestamp(t *testing.T) {
	s := New("test", nil, func() bool { return true })
	runtime := &observationRuntime{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.Observe(ctx, runtime, 10*time.Millisecond) }()
	deadline := time.Now().Add(time.Second)
	for runtime.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	v := s.Snapshot()
	if v.DockerReady || v.ObservedAt.IsZero() || len(v.Deployments) != 1 {
		t.Fatalf("bad failed observation: %+v", v)
	}
}

func TestServerRequiresLoopbackAndStops(t *testing.T) {
	for _, address := range []string{"0.0.0.0:9000", ":9000", "example.com:9000", "[::]:9000", "127.0.0.1:65536"} {
		if ValidateListen(address) == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := New("test", nil, nil)
	addr, done, err := s.Serve(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr.String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}
