package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
)

const (
	ExitOK      = 0
	ExitLocal   = 1
	ExitUsage   = 2
	ExitUnknown = 3
	ExitRemote  = 4
)

type response struct {
	OK        bool   `json:"ok"`
	RequestID string `json:"request_id,omitempty"`
	State     string `json:"state,omitempty"`
	Message   string `json:"message,omitempty"`
	Code      string `json:"code,omitempty"`
	Result    any    `json:"result,omitempty"`
}
type requestRecord struct {
	Environment  string   `json:"environment"`
	BrokerURL    string   `json:"broker_url"`
	CommandTopic string   `json:"command_topic"`
	EventTopic   string   `json:"event_topic"`
	Envelope     Envelope `json:"envelope"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("captain-compose", flag.ContinueOnError)
	global.SetOutput(stderr)
	configPath := global.String("config", "captain-compose.yaml", "operator configuration file")
	envName := global.String("environment", "", "named environment")
	jsonOutput := global.Bool("json", false, "write machine-readable JSON")
	global.Usage = func() {
		fmt.Fprintln(stderr, "usage: captain-compose [--config PATH] --environment NAME [--json] COMMAND [ARGS]")
		fmt.Fprintln(stderr, "commands: deploy remove plan status inspect wait result doctor revert")
	}
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	rest := global.Args()
	if len(rest) == 0 {
		global.Usage()
		return ExitUsage
	}
	if *envName == "" {
		printError(stderr, *jsonOutput, "environment is required", "usage")
		return ExitUsage
	}
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		printError(stderr, *jsonOutput, err.Error(), "local_failure")
		return ExitLocal
	}
	env, ok := cfg.Environments[*envName]
	if !ok {
		printError(stderr, *jsonOutput, "unknown environment", "usage")
		return ExitUsage
	}
	if err := env.ResolvePassword(); err != nil {
		printError(stderr, *jsonOutput, err.Error(), "local_failure")
		return ExitLocal
	}
	return runCommand(rest[0], rest[1:], *envName, env, *jsonOutput, stdout, stderr, Exchange)
}
func runCommand(command string, args []string, envName string, env Environment, jsonOut bool, stdout, stderr io.Writer, exchange exchangeFunc) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: captain-compose [global options] %s [options]\n", command)
		fs.PrintDefaults()
	}
	requestID := fs.String("request-id", "", "caller UUID; reuse only with identical request content")
	requestFile := fs.String("request-file", "", "write or replay an exact request envelope")
	waitFlag := fs.Bool("wait", true, "wait for the agent result")
	retryDefault := env.RetryInterval
	if retryDefault <= 0 {
		retryDefault = 10 * time.Second
	}
	retry := fs.Duration("retry-interval", retryDefault, "republish interval using the same request ID")
	expires := fs.Duration("expires", 5*time.Minute, "request expiry from now")
	expected := fs.String("expected-revision", "", "compare-and-swap revision")
	revision := fs.String("revision", "", "target revision for revert")
	allowRisk := fs.Bool("allow-data-risk", false, "acknowledge that revert does not roll back application data")
	createOnly := fs.Bool("create-only", false, "fail if the deployment already exists")
	flagArgs, pos := splitFlags(args)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	hasExpected := false
	expiresExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "expected-revision" {
			hasExpected = true
		}
		if f.Name == "expires" {
			expiresExplicit = true
		}
	})
	req := control.Request{}
	if hasExpected {
		v := *expected
		if v == "" && command != "deploy" && command != "plan" {
			printError(stderr, jsonOut, "an empty --expected-revision is only valid for deploy or plan", "usage")
			return ExitUsage
		}
		if v != "" && !validRevision(v) {
			printError(stderr, jsonOut, "--expected-revision must be a lowercase SHA-256", "usage")
			return ExitUsage
		}
		req.ExpectedRevision = &v
	}
	kind := command
	switch command {
	case "deploy", "plan":
		if len(pos) != 2 {
			usage(stderr, command)
			return ExitUsage
		}
		req.Name = pos[0]
		payload, err := os.ReadFile(pos[1])
		if err != nil {
			printError(stderr, jsonOut, fmt.Sprintf("read compose file: %v", err), "local_failure")
			return ExitLocal
		}
		if len(payload) > 1024*1024 {
			printError(stderr, jsonOut, "compose file exceeds the 1 MiB protocol limit", "usage")
			return ExitUsage
		}
		if err := deployment.ValidatePayload(payload); err != nil {
			printError(stderr, jsonOut, err.Error(), "usage")
			return ExitUsage
		}
		req.Payload = payload
		if command == "deploy" {
			kind = "create"
		}
	case "remove", "inspect":
		if len(pos) != 1 {
			usage(stderr, command)
			return ExitUsage
		}
		req.Name = pos[0]
		if command == "remove" {
			kind = "remove"
		}
	case "status":
		if len(pos) > 1 {
			usage(stderr, command)
			return ExitUsage
		}
		if len(pos) == 1 {
			req.Name = pos[0]
		}
	case "doctor":
		if len(pos) != 0 {
			usage(stderr, command)
			return ExitUsage
		}
	case "revert":
		if len(pos) != 1 || req.Name != "" {
			usage(stderr, command)
			return ExitUsage
		}
		req.Name = pos[0]
		// Revert requires an explicit target revision and acknowledgement that data is not rolled back.
		if *revision == "" {
			printError(stderr, jsonOut, "--revision is required", "usage")
			return ExitUsage
		}
		if !validRevision(*revision) {
			printError(stderr, jsonOut, "--revision must be a lowercase SHA-256", "usage")
			return ExitUsage
		}
		if !hasExpected || req.ExpectedRevision == nil || *req.ExpectedRevision == "" {
			printError(stderr, jsonOut, "revert requires --expected-revision", "usage")
			return ExitUsage
		}
		req.Revision = *revision
		req.AllowDataRisk = *allowRisk
		if !req.AllowDataRisk {
			printError(stderr, jsonOut, "revert requires --allow-data-risk acknowledgment", "usage")
			return ExitUsage
		}
	case "result", "wait":
		if len(pos) != 1 {
			usage(stderr, command)
			return ExitUsage
		}
		canonical, err := canonicalUUID(pos[0])
		if err != nil {
			printError(stderr, jsonOut, "request ID must be a UUID", "usage")
			return ExitUsage
		}
		req.ResultID = canonical
		kind = "result"
	default:
		printError(stderr, jsonOut, "unknown command", "usage")
		return ExitUsage
	}
	if req.Name != "" {
		if err := deployment.ValidateName(req.Name); err != nil {
			printError(stderr, jsonOut, err.Error(), "usage")
			return ExitUsage
		}
	}
	if *createOnly {
		if kind != "create" && kind != "plan" {
			printError(stderr, jsonOut, "--create-only is only valid for deploy or plan", "usage")
			return ExitUsage
		}
		if hasExpected {
			printError(stderr, jsonOut, "--create-only cannot be combined with --expected-revision", "usage")
			return ExitUsage
		}
		empty := ""
		req.ExpectedRevision = &empty
	}
	if hasExpected && kind != "create" && kind != "remove" && kind != "plan" && kind != "revert" {
		printError(stderr, jsonOut, "--expected-revision is only valid for deploy, remove, plan, and revert", "usage")
		return ExitUsage
	}
	if command != "revert" && (*revision != "" || *allowRisk) {
		printError(stderr, jsonOut, "--revision and --allow-data-risk are only valid for revert", "usage")
		return ExitUsage
	}
	if *requestID != "" {
		canonical, err := canonicalUUID(*requestID)
		if err != nil {
			printError(stderr, jsonOut, "--request-id must be a UUID", "usage")
			return ExitUsage
		}
		*requestID = canonical
	}
	if (command == "result" || command == "wait") && (*requestID != "" || *requestFile != "") {
		printError(stderr, jsonOut, "result queries always use fresh query IDs; --request-id and --request-file are not valid", "usage")
		return ExitUsage
	}
	if *requestID != "" && !expiresExplicit {
		*expires = 0
	}
	if *retry <= 0 {
		printError(stderr, jsonOut, "retry interval must be positive", "usage")
		return ExitUsage
	}
	if *expires < 0 || *expires > time.Hour {
		printError(stderr, jsonOut, "expiry must be between 0 and 1h", "usage")
		return ExitUsage
	}
	base, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(base, timeoutFor(env))
	defer cancel()
	var envelope Envelope
	recordLoaded := false
	if *requestFile != "" {
		if info, err := os.Lstat(*requestFile); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
				printError(stderr, jsonOut, "request file must be a regular file no larger than 2 MiB", "local_failure")
				return ExitLocal
			}
			f, openErr := os.Open(*requestFile)
			if openErr != nil {
				printError(stderr, jsonOut, "cannot open request file", "local_failure")
				return ExitLocal
			}
			openedInfo, statErr := f.Stat()
			currentInfo, lstatErr := os.Lstat(*requestFile)
			if statErr != nil || lstatErr != nil || !os.SameFile(info, openedInfo) || !os.SameFile(info, currentInfo) {
				_ = f.Close()
				printError(stderr, jsonOut, "request file changed while opening", "local_failure")
				return ExitLocal
			}
			b, readErr := io.ReadAll(io.LimitReader(f, 2*1024*1024+1))
			_ = f.Close()
			if readErr != nil || len(b) > 2*1024*1024 {
				printError(stderr, jsonOut, "cannot read request file", "local_failure")
				return ExitLocal
			}
			var record requestRecord
			if e := strictDecode(b, &record); e != nil {
				printError(stderr, jsonOut, "invalid request file", "local_failure")
				return ExitLocal
			}
			if record.Environment != envName || record.BrokerURL != env.BrokerURL || record.CommandTopic != env.CommandTopic || record.EventTopic != env.EventTopic {
				printError(stderr, jsonOut, "request file belongs to a different environment, broker, or topic pair", "usage")
				return ExitUsage
			}
			envelope = record.Envelope
			recordLoaded = true
			if envelope.Version != 2 || envelope.ID == "" || envelope.Type == "" {
				printError(stderr, jsonOut, "request file is missing v2 envelope fields", "local_failure")
				return ExitLocal
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			printError(stderr, jsonOut, "cannot read request file", "local_failure")
			return ExitLocal
		}
		if recordLoaded {
			canonical, err := canonicalUUID(envelope.ID)
			if err != nil || canonical != envelope.ID {
				printError(stderr, jsonOut, "request file contains an invalid request ID", "local_failure")
				return ExitLocal
			}
			if envelope.Type != kind || !sameJSON(envelope.Data, req) {
				printError(stderr, jsonOut, "request file content does not match this command; reuse it with the same operation inputs", "usage")
				return ExitUsage
			}
			if *requestID != "" && envelope.ID != *requestID {
				printError(stderr, jsonOut, "request file ID differs from --request-id", "usage")
				return ExitUsage
			}
		}
	}
	if envelope.ID == "" {
		expiry := *expires
		switch kind {
		case "plan", "status", "inspect", "doctor", "result":
			expiry = 0
		}
		envelope = NewEnvelope(kind, req, expiry)
		if *requestID != "" {
			envelope.ID = *requestID
		}
		if *requestFile != "" {
			record := requestRecord{Environment: envName, BrokerURL: env.BrokerURL, CommandTopic: env.CommandTopic, EventTopic: env.EventTopic, Envelope: envelope}
			if err := writeRequestRecord(*requestFile, record); err != nil {
				printError(stderr, jsonOut, "cannot persist request file", "local_failure")
				return ExitLocal
			}
		}
	}
	if envelope.Type != kind {
		printError(stderr, jsonOut, "request file command does not match requested operation", "usage")
		return ExitUsage
	}
	if command == "result" || command == "wait" {
		return queryResult(ctx, env, req.ResultID, command == "wait", *retry, jsonOut, stdout, stderr, exchange)
	}
	out, err := exchange(ctx, env, envelope, *retry, *waitFlag)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			printErrorWithID(stderr, jsonOut, "result is unknown; query or retry with the same request ID", "timeout", envelope.ID)
			return ExitUnknown
		}
		if errors.Is(err, context.Canceled) {
			printErrorWithID(stderr, jsonOut, "cancelled; result may be unknown", "cancelled", envelope.ID)
			return ExitUnknown
		}
		var unknown *UnknownError
		if errors.As(err, &unknown) {
			printErrorWithID(stderr, jsonOut, unknown.Error(), "unknown", envelope.ID)
			return ExitUnknown
		}
		printError(stderr, jsonOut, err.Error(), "local_failure")
		return ExitLocal
	}
	if !out.Received {
		writeResponse(stdout, jsonOut, response{OK: true, RequestID: envelope.ID, State: "broker_accepted", Message: "Broker accepted the request; agent execution is not confirmed."})
		return ExitOK
	}
	if out.Event.Version != 2 || out.Event.Action != kind {
		writeResponse(stdout, jsonOut, response{OK: false, RequestID: envelope.ID, State: "protocol_error", Code: "invalid_event", Message: "agent returned an incompatible operation result"})
		return ExitRemote
	}
	if command == "doctor" {
		checks, failed := doctorChecks(out.Event.Data)
		if failed {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: envelope.ID, State: "completed", Code: out.Event.Code, Message: "doctor reported failed or invalid checks", Result: checks})
			return ExitRemote
		}
	}
	if !out.Event.Success || (out.Event.Code != "" && out.Event.Code != "ok") {
		writeResponse(stdout, jsonOut, response{OK: false, RequestID: envelope.ID, State: "completed", Code: out.Event.Code, Message: out.Event.Message, Result: decodeData(out.Event.Data)})
		return ExitRemote
	}
	writeResponse(stdout, jsonOut, response{OK: out.Event.Success, RequestID: envelope.ID, State: "completed", Code: out.Event.Code, Message: out.Event.Message, Result: decodeData(out.Event.Data)})
	return ExitOK
}
func timeoutFor(e Environment) time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return 5 * time.Minute
}
func decodeData(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	return v
}
func sameJSON(a, b any) bool {
	left, err1 := json.Marshal(a)
	right, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(left) == string(right)
}
func strictDecode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}
func writeRequestRecord(path string, record requestRecord) error {
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(b)
	if writeErr == nil && n != len(b) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return syncRequestDirectory(filepath.Dir(path))
}
func validRevision(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
func canonicalUUID(s string) (string, error) {
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid UUID")
	}
	return id.String(), nil
}
func doctorChecks(data json.RawMessage) ([]control.Check, bool) {
	var checks []control.Check
	if len(data) == 0 || json.Unmarshal(data, &checks) != nil || len(checks) == 0 {
		return checks, true
	}
	for _, check := range checks {
		if !check.OK {
			return checks, true
		}
	}
	return checks, false
}
func doctorFailed(data json.RawMessage) bool { _, failed := doctorChecks(data); return failed }
func validSavedEvent(event Event, originalID string) bool {
	if event.RequestID != originalID {
		return false
	}
	switch event.Version {
	case 0, 1:
		return event.Action == "create" || event.Action == "delete"
	case 2:
		switch event.Action {
		case "create", "remove", "plan", "status", "inspect", "doctor", "revert":
			return true
		}
	}
	return false
}
func queryResult(ctx context.Context, env Environment, originalID string, poll bool, retry time.Duration, jsonOut bool, stdout, stderr io.Writer, exchange exchangeFunc) int {
	for {
		query := NewEnvelope("result", control.Request{ResultID: originalID}, 0)
		out, err := exchange(ctx, env, query, retry, true)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				printErrorWithID(stderr, jsonOut, "result is unknown or still pending", "timeout", originalID)
				return ExitUnknown
			}
			if errors.Is(err, context.Canceled) {
				printErrorWithID(stderr, jsonOut, "cancelled while waiting for result", "cancelled", originalID)
				return ExitUnknown
			}
			printError(stderr, jsonOut, err.Error(), "local_failure")
			return ExitLocal
		}
		ev := out.Event
		if ev.Version != 2 || ev.Action != "result" {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "protocol_error", Code: "invalid_event", Message: "agent returned an incompatible result query response"})
			return ExitRemote
		}
		if !ev.Success {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "completed", Code: ev.Code, Message: ev.Message})
			return ExitRemote
		}
		if ev.Code == "pending" {
			if !poll {
				writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "pending", Code: "pending", Message: ev.Message})
				return ExitUnknown
			}
			t := time.NewTimer(retry)
			select {
			case <-ctx.Done():
				t.Stop()
				printErrorWithID(stderr, jsonOut, "result is still pending; request ID can be queried again", "timeout", originalID)
				return ExitUnknown
			case <-t.C:
			}
			continue
		}
		if ev.Code == "unknown" {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "unknown", Code: ev.Code, Message: ev.Message})
			return ExitUnknown
		}
		if ev.Code != "executed" && ev.Code != "delivered" {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "protocol_error", Code: "invalid_result", Message: "agent returned an invalid result status"})
			return ExitRemote
		}
		var d struct {
			Result *Event `json:"result"`
			Status string `json:"status"`
		}
		if len(ev.Data) == 0 || strictDecode(ev.Data, &d) != nil || d.Result == nil || d.Status != ev.Code || !validSavedEvent(*d.Result, originalID) {
			writeResponse(stdout, jsonOut, response{OK: false, RequestID: originalID, State: "protocol_error", Code: "invalid_result", Message: "agent returned a malformed or mismatched saved result"})
			return ExitRemote
		}
		success, code, message := d.Result.Success, d.Result.Code, d.Result.Message
		if d.Result.Action == "doctor" && doctorFailed(d.Result.Data) {
			success = false
			code = "doctor_failed"
			message = "doctor reported failed or invalid checks"
		}
		writeResponse(stdout, jsonOut, response{OK: success, RequestID: originalID, State: d.Status, Code: code, Message: message, Result: d.Result})
		if !success || (code != "" && code != "ok") {
			return ExitRemote
		}
		return ExitOK
	}
}

func splitFlags(args []string) ([]string, []string) {
	flags, positional := make([]string, 0, len(args)), make([]string, 0, len(args))
	bools := map[string]bool{"wait": true, "allow-data-risk": true, "create-only": true}
	valueNext := false
	for _, arg := range args {
		if valueNext {
			flags = append(flags, arg)
			valueNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
			if !bools[name] && !strings.Contains(arg, "=") {
				valueNext = true
			}
		} else {
			positional = append(positional, arg)
		}
	}
	return flags, positional
}
func writeResponse(w io.Writer, jsonOut bool, r response) {
	if jsonOut {
		_ = json.NewEncoder(w).Encode(r)
		return
	}
	label := r.State
	if label == "" {
		label = "error"
	}
	fmt.Fprintf(w, "%s", label)
	if r.RequestID != "" {
		fmt.Fprintf(w, " (request %s)", r.RequestID)
	}
	if r.Message != "" {
		fmt.Fprintf(w, ": %s", r.Message)
	}
	if r.Result != nil {
		b, _ := json.MarshalIndent(r.Result, "", "  ")
		fmt.Fprintf(w, "\n%s", b)
	}
	fmt.Fprintln(w)
}
func printError(w io.Writer, jsonOut bool, msg, code string) {
	if jsonOut {
		_ = json.NewEncoder(w).Encode(response{OK: false, Code: code, Message: msg})
		return
	}
	fmt.Fprintln(w, msg)
}
func printErrorWithID(w io.Writer, jsonOut bool, msg, code, id string) {
	if jsonOut {
		_ = json.NewEncoder(w).Encode(response{OK: false, RequestID: id, Code: code, Message: msg})
		return
	}
	fmt.Fprintf(w, "request_id=%s: %s\n", id, msg)
}
func usage(w io.Writer, cmd string) { fmt.Fprintf(w, "usage: captain-compose %s [options]", cmd) }
