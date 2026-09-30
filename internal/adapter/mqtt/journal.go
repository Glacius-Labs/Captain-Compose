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
const maxReceipts = 10000
const receiptRetention = 7 * 24 * time.Hour

var ErrIDConflict = errors.New("request id reused with different content")

type record struct {
	ID        string            `json:"id"`
	Hash      string            `json:"hash"`
	Sequence  uint64            `json:"sequence"`
	Received  time.Time         `json:"received"`
	Command   *Envelope         `json:"command,omitempty"`
	Event     *deployment.Event `json:"event,omitempty"`
	Completed *time.Time        `json:"completed,omitempty"`
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
	if rec.Sequence == 0 || rec.Received.IsZero() || (rec.Completed != nil && rec.Event == nil) {
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
	for id, r := range j.records {
		if r.Completed != nil && time.Since(*r.Completed) > receiptRetention {
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
	if err := j.prune(); err != nil {
		return err
	}
	b, _ := json.Marshal(e)
	hash := sha256.Sum256(b)
	digest := hex.EncodeToString(hash[:])
	if existing, ok := j.records[e.ID]; ok {
		if existing.Hash != digest {
			return ErrIDConflict
		}
		return nil
	}
	pending := 0
	for _, r := range j.records {
		if r.Completed == nil {
			pending++
		}
	}
	if pending >= maxPending || len(j.records) >= maxReceipts {
		return fmt.Errorf("journal capacity reached; wait for processing or receipt retention expiry")
	}
	if j.sequence == ^uint64(0) {
		return fmt.Errorf("journal sequence exhausted")
	}
	seq := j.sequence + 1
	if err := j.save(record{ID: e.ID, Hash: digest, Sequence: seq, Received: time.Now().UTC(), Command: &e}); err != nil {
		return err
	}
	j.sequence = seq
	return nil
}

func (j *journal) next() (record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var pending []record
	for _, r := range j.records {
		if r.Completed == nil {
			pending = append(pending, r)
		}
	}
	sort.Slice(pending, func(i, k int) bool { return pending[i].Sequence < pending[k].Sequence })
	if len(pending) == 0 {
		return record{}, false
	}
	return pending[0], true
}

func (j *journal) update(rec record) error { j.mu.Lock(); defer j.mu.Unlock(); return j.save(rec) }
