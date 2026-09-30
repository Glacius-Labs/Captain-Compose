package mqtt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

const maxPending = 128
const reservedQueryCapacity = 16
const maxReceipts = 10000
const receiptRetention = 7 * 24 * time.Hour
const maxQueryReceipts = 1000
const queryReceiptRetention = 10 * time.Minute
const MaxEventBytes = 256 * 1024

var ErrCapacity = errors.New("journal capacity reached")

var ErrIDConflict = errors.New("request id reused with different content")

type record struct {
	ID        string            `json:"id"`
	Hash      string            `json:"hash"`
	Sequence  uint64            `json:"sequence"`
	Received  time.Time         `json:"received"`
	Command   *Envelope         `json:"command,omitempty"`
	Event     *deployment.Event `json:"event,omitempty"`
	Completed *time.Time        `json:"completed,omitempty"`
	Started   *time.Time        `json:"started,omitempty"`
	Delivered *time.Time        `json:"delivered,omitempty"`
	Query     bool              `json:"query,omitempty"`
}

type journal struct {
	mu       sync.Mutex
	root     *os.Root
	records  map[string]record
	sequence uint64
	syncDir  func(*os.Root) error
	failed   error
}

func openJournal(dir string) (*journal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	j := &journal{root: r, records: map[string]record{}, syncDir: syncDirectory}
	f, err := r.Open(".")
	if err != nil {
		r.Close()
		return nil, err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		r.Close()
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := r.ReadFile(entry.Name())
		if err != nil {
			r.Close()
			return nil, err
		}
		var rec record
		if err := json.Unmarshal(b, &rec); err != nil || rec.ID == "" || journalName(rec.ID) != entry.Name() || (rec.Command == nil && rec.Event == nil) {
			r.Close()
			return nil, fmt.Errorf("invalid journal record %s; restore state from backup", entry.Name())
		}
		if err := validateRecord(rec); err != nil {
			r.Close()
			return nil, fmt.Errorf("invalid journal record %s: %w", entry.Name(), err)
		}
		j.records[rec.ID] = rec
	}
	if err := j.prune(); err != nil {
		r.Close()
		return nil, err
	}
	sequences := make(map[uint64]string, len(j.records))
	for id, rec := range j.records {
		if prior, exists := sequences[rec.Sequence]; exists {
			r.Close()
			return nil, fmt.Errorf("duplicate journal sequence %d in records %s and %s", rec.Sequence, prior, id)
		}
		sequences[rec.Sequence] = id
		if rec.Sequence > j.sequence {
			j.sequence = rec.Sequence
		}
	}
	return j, nil
}

func validateRecord(rec record) error {
	if rec.Sequence == 0 || rec.Received.IsZero() || (rec.Completed != nil && rec.Event == nil) || (rec.Delivered != nil && rec.Event == nil) {
		return fmt.Errorf("missing receipt metadata")
	}
	if rec.Event != nil && (rec.Event.RequestID != rec.ID || rec.Event.ID.String() == "00000000-0000-0000-0000-000000000000" || rec.Event.Action == "") {
		return fmt.Errorf("invalid result metadata")
	}
	if rec.Command != nil {
		b, err := json.Marshal(rec.Command)
		if err != nil {
			return err
		}
		e, err := Decode(b)
		if err != nil || e.ID != rec.ID {
			return fmt.Errorf("invalid stored command")
		}
		hash := sha256.Sum256(b)
		if hex.EncodeToString(hash[:]) != rec.Hash {
			return fmt.Errorf("command checksum mismatch")
		}
	}
	return nil
}

func journalName(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:]) + ".json"
}

func (j *journal) save(rec record) (err error) {
	if j.failed != nil {
		return fmt.Errorf("journal is unavailable after a persistence failure: %w", j.failed)
	}
	defer func() {
		if err != nil {
			j.failed = err
		}
	}()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	name := journalName(rec.ID)
	f, err := j.root.OpenFile(name+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = j.root.Rename(name+".tmp", name); err != nil {
		return err
	}
	if err = j.syncDir(j.root); err != nil {
		return err
	}
	j.records[rec.ID] = rec
	return nil
}

func (j *journal) prune() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.pruneLocked()
}

func (j *journal) pruneLocked() error {
	for id, r := range j.records {
		retainedAt := r.Delivered
		if retainedAt == nil && (r.Event == nil || r.Event.Version != 2) {
			retainedAt = r.Completed
		}
		retention := receiptRetention
		if r.Query {
			retention = queryReceiptRetention
		}
		if retainedAt != nil && time.Since(*retainedAt) > retention {
			if err := j.root.Remove(journalName(id)); err != nil {
				j.failed = err
				return err
			}
			delete(j.records, id)
		}
	}
	return nil
}

func (j *journal) enqueue(e Envelope) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failed != nil {
		return fmt.Errorf("journal is unavailable after a persistence failure: %w", j.failed)
	}
	if err := j.pruneLocked(); err != nil {
		return err
	}
	b, _ := json.Marshal(e)
	hash := sha256.Sum256(b)
	digest := hex.EncodeToString(hash[:])
	if existing, ok := j.records[e.ID]; ok {
		if existing.Hash != digest {
			return ErrIDConflict
		}
		// Version 2 retries can recover a result after the controller missed its
		// publication. Version 1 keeps its original no-republish behavior.
		if e.Version == 2 && existing.Event != nil && existing.Delivered != nil {
			if j.pendingCount(existing.Query) >= j.capacity(existing.Query) {
				return ErrCapacity
			}
			existing.Delivered = nil
			return j.save(existing)
		}
		return nil
	}
	query := e.Version == 2 && isQueryAction(e.Type)
	if j.pendingCount(query) >= j.capacity(query) {
		return ErrCapacity
	}
	receipts := 0
	queryReceipts := 0
	for _, r := range j.records {
		if r.Query {
			queryReceipts++
		} else {
			receipts++
		}
	}
	if (!query && receipts >= maxReceipts) || (query && queryReceipts >= maxQueryReceipts) {
		return ErrCapacity
	}
	if j.sequence == ^uint64(0) {
		return fmt.Errorf("journal sequence exhausted")
	}
	seq := j.sequence + 1
	if err := j.save(record{ID: e.ID, Hash: digest, Sequence: seq, Received: time.Now().UTC(), Command: &e, Query: query}); err != nil {
		return err
	}
	j.sequence = seq
	return nil
}

func (j *journal) capacity(query bool) int {
	if query {
		return reservedQueryCapacity
	}
	return maxPending - reservedQueryCapacity
}

func (j *journal) pendingCount(query bool) int {
	n := 0
	for _, r := range j.records {
		if r.Query == query && (r.Event == nil || (r.Delivered == nil && (r.Event.Version == 2 || r.Completed == nil))) {
			n++
		}
	}
	return n
}

func isQueryAction(action string) bool {
	switch action {
	case "plan", "status", "inspect", "doctor", "result":
		return true
	default:
		return false
	}
}

func (j *journal) nextExecution() (record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var pending []record
	for _, r := range j.records {
		if r.Event == nil && !r.Query {
			pending = append(pending, r)
		}
	}
	sort.Slice(pending, func(i, k int) bool { return pending[i].Sequence < pending[k].Sequence })
	if len(pending) == 0 {
		return record{}, false
	}
	return pending[0], true
}

func (j *journal) nextQuery() (record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var pending []record
	for _, r := range j.records {
		if r.Event == nil && r.Query {
			pending = append(pending, r)
		}
	}
	sort.Slice(pending, func(i, k int) bool { return pending[i].Sequence < pending[k].Sequence })
	if len(pending) == 0 {
		return record{}, false
	}
	return pending[0], true
}

// next is kept for legacy journal tests and callers; execution selection is unchanged.
func (j *journal) next() (record, bool) { return j.nextExecution() }

func (j *journal) nextOutbox() (record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var pending []record
	for _, r := range j.records {
		if r.Event != nil && r.Delivered == nil && (r.Event.Version == 2 || r.Completed == nil) {
			pending = append(pending, r)
		}
	}
	sort.Slice(pending, func(i, k int) bool { return pending[i].Sequence < pending[k].Sequence })
	if len(pending) == 0 {
		return record{}, false
	}
	return pending[0], true
}

func (j *journal) find(id string) (record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.records[id]
	return r, ok
}

func (j *journal) markStarted(id string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[id]
	if !ok {
		return fmt.Errorf("request disappeared from journal")
	}
	if rec.Started == nil {
		rec.Started = &now
	}
	return j.save(rec)
}

func (j *journal) saveEvent(id string, event deployment.Event, completed *time.Time, pruneCommand bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[id]
	if !ok {
		return fmt.Errorf("request disappeared from journal")
	}
	rec.Event = &event
	if completed != nil {
		rec.Completed = completed
	}
	if pruneCommand {
		rec.Command = nil
	}
	return j.save(rec)
}

func (j *journal) markDelivered(id string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	rec, ok := j.records[id]
	if !ok || rec.Event == nil {
		return fmt.Errorf("result disappeared from journal")
	}
	rec.Delivered = &now
	if rec.Event.Version != 2 {
		rec.Completed = &now
		rec.Command = nil
	}
	return j.save(rec)
}

func (j *journal) update(rec record) error { j.mu.Lock(); defer j.mu.Unlock(); return j.save(rec) }
