package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/app"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
)

type StatsSnapshot struct {
	Pending              int
	Outbox               int
	Receipts             int
	OldestPendingSeconds float64
	OldestOutboxSeconds  float64
	LastCompleted        time.Time
}

type Listener struct {
	topic            string
	runtime          deployment.Runtime
	v2               control.Runtime
	publisher        deployment.Publisher
	journal          *journal
	wake             chan struct{}
	failures         chan error
	ingressMu        sync.Mutex
	deferred         []deferredCommand
	deferredOverflow bool
}

const deferredCapacity = 16

type deferredCommand struct {
	message  paho.Message
	envelope Envelope
	query    bool
	received time.Time
}

var ErrDeferredOverflow = errors.New("deferred MQTT ingress queue overflow; restart after queued commands are persisted")

func NewListener(topic, dir string, rt deployment.Runtime, pub deployment.Publisher) (*Listener, error) {
	j, err := openJournal(dir)
	if err != nil {
		return nil, err
	}
	l := &Listener{topic: topic, runtime: rt, publisher: pub, journal: j, wake: make(chan struct{}, 1), failures: make(chan error, 2)}
	l.v2, _ = rt.(control.Runtime)
	return l, nil
}

func (l *Listener) Close() error { return l.journal.root.Close() }
func (l *Listener) fail(err error) {
	select {
	case l.failures <- err:
	default:
	}
}

func (l *Listener) Stats() StatsSnapshot {
	now := time.Now()
	s := StatsSnapshot{}
	l.journal.mu.Lock()
	for _, r := range l.journal.records {
		if r.Event == nil {
			s.Pending++
			age := now.Sub(r.Received).Seconds()
			if age > s.OldestPendingSeconds {
				s.OldestPendingSeconds = age
			}
			continue
		}
		if r.Event.Version == 2 && r.Delivered == nil || r.Event.Version == 0 && r.Completed == nil {
			s.Outbox++
			age := now.Sub(r.Received).Seconds()
			if age > s.OldestOutboxSeconds {
				s.OldestOutboxSeconds = age
			}
		}
		if !r.Query {
			if r.Delivered != nil || (r.Event.Version != 2 && r.Completed != nil) {
				s.Receipts++
			}
			at := r.Completed
			if at == nil {
				at = r.Delivered
			}
			if at != nil && at.After(s.LastCompleted) {
				s.LastCompleted = *at
			}
		}
	}
	l.journal.mu.Unlock()
	l.ingressMu.Lock()
	for _, item := range l.deferred {
		s.Pending++
		age := now.Sub(item.received).Seconds()
		if age > s.OldestPendingSeconds {
			s.OldestPendingSeconds = age
		}
	}
	l.ingressMu.Unlock()
	return s
}

// Subscribe runs on every connection; durable command work runs outside Paho callbacks.
func (l *Listener) Subscribe(client paho.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Wait(ctx, client.Subscribe(l.topic, 1, l.HandleMessage)); err != nil {
		l.fail(fmt.Errorf("MQTT subscribe: %w", err))
		return
	}
	slog.Info("MQTT subscription ready", "topic", l.topic)
}

func (l *Listener) HandleMessage(_ paho.Client, msg paho.Message) {
	l.ingressMu.Lock()
	defer l.ingressMu.Unlock()
	if msg.Retained() || msg.Qos() != 1 {
		slog.Warn("Rejected command: require non-retained QoS 1")
		msg.Ack()
		return
	}
	e, err := Decode(msg.Payload())
	if err != nil {
		slog.Warn("Rejected command", "error", err)
		msg.Ack()
		return
	}
	query := e.Version == 2 && isQueryAction(e.Type)
	if (query && l.hasDeferred(true)) || (!query && l.hasDeferred(false)) {
		l.deferMessage(msg, e, query)
		return
	}
	if err = l.journal.enqueue(e); err != nil {
		if errors.Is(err, ErrIDConflict) {
			slog.Warn("Rejected conflicting request ID", "request_id", e.ID)
			msg.Ack()
			return
		}
		if errors.Is(err, ErrCapacity) {
			slog.Warn("Command deferred without acknowledgement because the durable journal is full", "request_id", e.ID)
			l.deferMessage(msg, e, query)
			return
		}
		l.fail(fmt.Errorf("persist command: %w", err))
		return
	}
	msg.Ack()
	l.signal()
}

func (l *Listener) hasDeferred(query bool) bool {
	for _, item := range l.deferred {
		if item.query == query {
			return true
		}
	}
	return false
}

func (l *Listener) deferMessage(msg paho.Message, e Envelope, query bool) {
	if len(l.deferred) == deferredCapacity {
		l.deferredOverflow = true
		slog.Warn("Deferred MQTT ingress queue full; message remains unacknowledged for broker redelivery", "request_id", e.ID)
		return
	}
	l.deferred = append(l.deferred, deferredCommand{message: msg, envelope: e, query: query, received: time.Now().UTC()})
	l.signal()
}

func (l *Listener) flushDeferred() (bool, error) {
	l.ingressMu.Lock()
	defer l.ingressMu.Unlock()
	if len(l.deferred) == 0 {
		return false, nil
	}
	index := 0
	for i, queued := range l.deferred {
		if queued.query {
			index = i
			break
		}
	}
	flushAt := func(index int) (bool, error) {
		item := l.deferred[index]
		if err := l.journal.enqueue(item.envelope); err != nil {
			if errors.Is(err, ErrCapacity) {
				return false, nil
			}
			if !errors.Is(err, ErrIDConflict) {
				return false, err
			}
			l.deferred = append(l.deferred[:index], l.deferred[index+1:]...)
			item.message.Ack()
			slog.Warn("Rejected conflicting deferred request ID", "request_id", item.envelope.ID)
		} else {
			l.deferred = append(l.deferred[:index], l.deferred[index+1:]...)
			item.message.Ack()
			l.signal()
		}
		if len(l.deferred) == 0 && l.deferredOverflow {
			return true, ErrDeferredOverflow
		}
		return true, nil
	}
	done, err := flushAt(index)
	if done || err != nil || !l.deferred[index].query {
		return done, err
	}
	// A saturated query quota must not hold up queued mutations that still have
	// their reserved capacity available.
	for i, queued := range l.deferred {
		if !queued.query {
			return flushAt(i)
		}
	}
	return false, nil
}

func (l *Listener) signal() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Start owns two bounded workers: one serial executor and one independent outbox.
func (l *Listener) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{}, 3)
	go func() { defer func() { done <- struct{}{} }(); l.executeLoop(workerCtx) }()
	go func() { defer func() { done <- struct{}{} }(); l.deliveryLoop(workerCtx) }()
	go func() { defer func() { done <- struct{}{} }(); l.queryLoop(workerCtx) }()
	finished := 0
	ctxDone := ctx.Done()
	for finished < 3 {
		select {
		case <-ctxDone:
			cancel()
			ctxDone = nil
		case <-done:
			finished++
		case err := <-l.failures:
			cancel()
			for finished < 3 {
				<-done
				finished++
			}
			return err
		}
	}
	return nil
}

func (l *Listener) executeLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if rec, ok := l.journal.nextExecution(); ok {
			event, err := l.execute(ctx, *rec.Command)
			if err != nil {
				l.fail(err)
				return
			}
			if ctx.Err() != nil {
				// Interrupted commands remain pending. V2's durable started marker
				// allows a resumed mutation to pass its original expiry check.
				return
			}
			event.RequestID = rec.ID
			event = boundV2Event(event)
			var completed *time.Time
			if event.Version == 2 {
				now := time.Now().UTC()
				completed = &now
			}
			if err := l.journal.saveEvent(rec.ID, event, completed, event.Version == 2); err != nil {
				l.fail(fmt.Errorf("persist result: %w", err))
				return
			}
			l.signal()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		case <-ticker.C:
		}
	}
}

func (l *Listener) queryLoop(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := l.flushDeferred(); errors.Is(err, ErrDeferredOverflow) {
			l.fail(err)
			return
		} else if err != nil {
			l.fail(fmt.Errorf("persist deferred command: %w", err))
			return
		}
		if err := l.journal.prune(); err != nil {
			l.fail(fmt.Errorf("prune command receipts: %w", err))
			return
		}
		if rec, ok := l.journal.nextQuery(); ok {
			event, err := l.execute(ctx, *rec.Command)
			if err != nil {
				l.fail(err)
				return
			}
			if ctx.Err() != nil {
				return
			}
			event.RequestID = rec.ID
			event = boundV2Event(event)
			now := time.Now().UTC()
			if err := l.journal.saveEvent(rec.ID, event, &now, true); err != nil {
				l.fail(fmt.Errorf("persist query result: %w", err))
				return
			}
			l.signal()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		case <-ticker.C:
		}
	}
}

func (l *Listener) deliveryLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if rec, ok := l.journal.nextOutbox(); ok {
			if err := l.publisher.Publish(ctx, *rec.Event); err == nil {
				now := time.Now().UTC()
				if err := l.journal.markDelivered(rec.ID, now); err != nil {
					l.fail(fmt.Errorf("persist receipt: %w", err))
					return
				}
				slog.Debug("Command result delivered", "request_id", rec.ID, "success", rec.Event.Success)
				continue
			} else if ctx.Err() == nil {
				slog.Warn("Event delivery pending; will retry", "request_id", rec.ID, "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		case <-ticker.C:
		}
	}
}

type capturePublisher struct{ event deployment.Event }

func (p *capturePublisher) Publish(_ context.Context, e deployment.Event) error {
	p.event = e
	return nil
}

func (l *Listener) execute(ctx context.Context, e Envelope) (deployment.Event, error) {
	if e.Version == 2 {
		return l.executeV2(ctx, e)
	}
	p := &capturePublisher{}
	application := app.New(l.runtime, p)
	switch e.Type {
	case TypeCreate:
		var c create.Command
		_ = json.Unmarshal(e.Data, &c)
		if err := application.Deployment.Create.Handle(ctx, c); err != nil && p.event.Action == "" {
			return deployment.NewCreationFailedEvent(c.Name, err), nil
		}
	case TypeRemove:
		var c remove.Command
		_ = json.Unmarshal(e.Data, &c)
		if err := application.Deployment.Remove.Handle(ctx, c); err != nil && p.event.Action == "" {
			return deployment.NewRemovalFailedEvent(c.Name, err), nil
		}
	}
	return p.event, nil
}

func (l *Listener) executeV2(ctx context.Context, e Envelope) (deployment.Event, error) {
	var req control.Request
	_ = json.Unmarshal(e.Data, &req)
	event := deployment.Event{Version: 2, ID: uuid.New(), Timestamp: time.Now().UTC(), Action: e.Type, Name: req.Name, Success: true, Code: "ok", Message: "Operation completed"}
	req.RequestID = e.ID
	if e.Type == "result" {
		target, found := l.journal.find(req.ResultID)
		if found && target.Event != nil && target.Event.Action == "result" {
			return v2Failure(event, "invalid_result_target", "A result lookup cannot reference another result lookup"), nil
		}
		status := "unknown"
		var saved any
		if found {
			switch {
			case target.Event == nil:
				status = "pending"
			case target.Delivered != nil || (target.Event.Version != 2 && target.Completed != nil):
				status = "delivered"
			default:
				status = "executed"
			}
			if target.Event != nil && target.Event.Action != "result" {
				saved = sanitizedSavedEvent(*target.Event)
			}
		}
		event.Action = "result"
		event.Code = status
		event.Data = map[string]any{"status": status, "result": safeValue(saved)}
		event.Message = "Request result lookup completed"
		return event, nil
	}
	if l.v2 == nil {
		return v2Failure(event, "runtime_unavailable", "Version 2 runtime is unavailable"), nil
	}
	mutating := e.Type == "create" || e.Type == "remove" || e.Type == "revert"
	newlyStarted := false
	if mutating && e.ExpiresAt != nil {
		if rec, found := l.journal.find(e.ID); found && rec.Started == nil && time.Now().After(*e.ExpiresAt) {
			return v2Failure(event, "expired", "Request expired before execution"), nil
		}
	}
	if mutating {
		rec, found := l.journal.find(e.ID)
		if found && rec.Started == nil {
			now := time.Now().UTC()
			if e.ExpiresAt != nil && now.After(*e.ExpiresAt) {
				return v2Failure(event, "expired", "Request expired before execution"), nil
			}
			if err := l.journal.markStarted(e.ID, now); err != nil {
				return deployment.Event{}, fmt.Errorf("persist execution start for %s: %w", e.ID, err)
			}
			newlyStarted = true
		}
	}
	if newlyStarted && e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt) {
		return v2Failure(event, "expired", "Request expired before execution"), nil
	}
	data, err := l.v2.Execute(ctx, e.Type, req)
	if err != nil {
		message := "Operation failed"
		var classified *control.Error
		if errors.As(err, &classified) {
			message = classified.Message
		}
		return v2Failure(event, control.ErrorCode(err), message), nil
	}
	if isQueryAction(e.Type) {
		event.Data = safeValue(data)
	}
	return event, nil
}

func sanitizedSavedEvent(event deployment.Event) deployment.Event {
	event.Labels = nil
	if event.Version == 2 && isQueryAction(event.Action) {
		return event
	}
	event.Data = nil
	if event.Success {
		event.Message = "Saved operation succeeded"
	} else {
		event.Message = "Saved operation failed"
	}
	return event
}

func v2Failure(event deployment.Event, code, message string) deployment.Event {
	event.Success = false
	event.Code = code
	event.Message = message
	return event
}

func safeValue(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var decoded any
	if json.Unmarshal(b, &decoded) != nil {
		return nil
	}
	return decoded
}

func boundV2Event(event deployment.Event) deployment.Event {
	if event.Version != 2 {
		return event
	}
	b, err := json.Marshal(event)
	if err == nil && len(b) <= MaxEventBytes {
		return event
	}
	event.Data = nil
	event.Labels = nil
	if isQueryAction(event.Action) {
		event.Success = false
		event.Code = "response_too_large"
		event.Message = "Response too large; narrow the query, for example by inspecting one deployment"
	} else {
		event.Code = "result_omitted"
		event.Message = "Operation completed; result data omitted because it exceeded the event size limit"
	}
	return event
}
