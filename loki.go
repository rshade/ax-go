package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Loki push configuration and label key constants. Using named constants for all
// numeric and string values that appear more than once satisfies the goconst and
// mnd linters and makes tuning parameters easy to locate.
const (
	// levelUnknown is returned by extractLevel when the zerolog JSON line does
	// not contain a recognisable "level" field, or when the field value is empty.
	levelUnknown = "unknown"

	// lokiFlushTimeout caps the wait in drain and the final push issued by
	// flushRemaining so process shutdown cannot block indefinitely; entries
	// still in flight after this are dropped.
	lokiFlushTimeout = 2 * time.Second

	// lokiPostTimeout bounds each individual HTTP request to the Loki push API.
	lokiPostTimeout = 10 * time.Second

	// lokiBatchSize triggers a push flush when the in-memory batch reaches this
	// many entries (before the 1-second ticker fires).
	lokiBatchSize = 100

	// lokiChannelCap is the capacity of the per-writer channel. Write calls that
	// arrive when the channel is full are dropped silently (FR-004).
	lokiChannelCap = 256

	// Loki stream label keys — the five permitted low-cardinality fields.
	// The environment/application/host/version keys share their string values
	// with labelField* in logger.go (defined alongside Labels) so that renaming
	// a label field stays a single-file change. trace_id, span_id, and all
	// high-cardinality fields must never appear as stream keys (FR-009).
	lokiLabelEnvironment = labelFieldEnvironment
	lokiLabelApplication = labelFieldApplication
	lokiLabelHost        = labelFieldHost
	lokiLabelVersion     = labelFieldVersion
	lokiLabelLevel       = "level"
)

// lokiStream is one entry in a Loki push body, grouping log entries that share
// the same label set. Level is the per-entry discriminator: entries are grouped
// into one stream per distinct level value within a batch.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// lokiPushBody is the JSON payload sent to POST /loki/api/v1/push.
// It matches the Loki HTTP API v1 push format (Loki ≥ 2.0).
type lokiPushBody struct {
	Streams []lokiStream `json:"streams"`
}

// lokiEntry is one queued log line plus the wall-clock time it was enqueued.
// Capturing the timestamp at enqueue (in Write) rather than at flush keeps each
// entry's Loki timestamp close to the actual event time, preserving ordering
// and time-range query fidelity within a stream.
type lokiEntry struct {
	ts   int64 // unix nanoseconds, captured at enqueue time
	line string
}

// lokiStreamKey is the full Loki stream grouping key. Entries with different
// low-cardinality labels must not be grouped into the same Loki stream, even
// when they share a zerolog level.
type lokiStreamKey struct {
	environment string
	application string
	host        string
	version     string
	level       string
}

// labelPair is one sanctioned (key, value) label pair declared through
// WithLoggerLabels or Logger.WithLabels. Only exact pairs in this set may be
// promoted from a log line into the Loki stream label set (FR-009).
type labelPair struct {
	key   string
	value string
}

// lokiWriter is a non-blocking logSink that queues zerolog log lines in a
// bounded channel and a background goroutine batches them into Loki push
// requests. Write always returns (len(p), nil) so callers are never blocked;
// push failures are surfaced as bounded diagnostics on the configured writer
// and the affected entries are dropped (fail-open, FR-005). Constitution
// Principle IX: resource safety — TLS is never skipped (ax.HTTPClient()) and
// buffer is bounded at 256 entries.
type lokiWriter struct {
	pushURL   string // resolved <AX_LOKI_URL>/loki/api/v1/push endpoint
	authToken string
	// errorWriter points at the loggerConfig writer field so push diagnostics
	// resolve the configured writer lazily at emit time: LoggerOption order
	// between WithLokiFromEnv and WithLoggerWriter does not matter. Nil only in
	// tests that construct lokiWriter directly; diagf guards it.
	errorWriter   *io.Writer
	ch            chan lokiEntry
	flushRequests chan chan struct{}
	client        *http.Client
	done          chan struct{}

	// labelMu guards sanctionedLabels.
	labelMu sync.RWMutex
	// sanctionedLabels is the set of label pairs declared through
	// WithLoggerLabels or Logger.WithLabels. streamKeyFromLine promotes a JSON
	// field into the stream label set only when its exact (key, value) pair is
	// present, so a payload field that merely reuses a label key name (e.g.
	// .Str("host", req.Host)) can never become a high-cardinality label
	// (FR-009, Constitution Principle VIII).
	sanctionedLabels map[labelPair]struct{}
}

// Write queues p for asynchronous delivery to Loki. It is non-blocking: if the
// internal channel is full the entry is silently dropped. Write always returns
// (len(p), nil) so callers (including zerolog's io.Writer path) are never
// blocked or returned an error. The bytes are converted to a string (a single
// copy) before queuing because the caller may reuse the underlying buffer; the
// resulting string is reused verbatim in the Loki push payload.
//
// Exit code mapping: Write never returns an error; failures are silently dropped
// to preserve FR-004 (non-blocking) and FR-005 (fail-open) contracts.
func (lw *lokiWriter) Write(p []byte) (int, error) {
	entry := lokiEntry{ts: time.Now().UnixNano(), line: string(p)}
	select {
	case lw.ch <- entry:
	default:
		// Channel full: drop silently (FR-004/FR-005).
	}
	return len(p), nil
}

// drain signals the background goroutine to flush buffered entries without
// stopping it, then waits for the flush to finish. The wait respects the
// caller's context deadline but is always capped at 2 seconds to avoid hanging
// the process during shutdown. Any entries still in-flight after the deadline
// are dropped. Returns nil in all cases (network failures during flush are
// already dropped by postBatch).
func (lw *lokiWriter) drain(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx2, cancel := context.WithTimeout(ctx, lokiFlushTimeout)
	defer cancel()

	ack := make(chan struct{})
	select {
	case <-lw.done:
		return nil
	case lw.flushRequests <- ack:
	case <-ctx2.Done():
		return nil
	}

	select {
	case <-ack:
	case <-lw.done:
	case <-ctx2.Done():
		// Deadline or cancellation: remaining entries dropped.
	}
	return nil
}

// run is the background goroutine that batches log lines and posts them to Loki.
// It exits after draining remaining channel entries when the supplied context
// (the one passed to NewLogger) is cancelled. The context bounds the goroutine
// to the logger's lifetime so it does not leak when the caller forgets to call
// Flush.
func (lw *lokiWriter) run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer close(lw.done)

	batch := make([]lokiEntry, 0, lokiBatchSize)

	for {
		select {
		case entry := <-lw.ch:
			batch = append(batch, entry)
			if len(batch) >= lokiBatchSize {
				lw.postBatch(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				lw.postBatch(ctx, batch)
				batch = batch[:0]
			}
		case ack := <-lw.flushRequests:
			batch = lw.flushRemaining(batch)
			close(ack)
		case <-ctx.Done():
			lw.flushRemaining(batch)
			return
		}
	}
}

// flushRemaining drains any entries still buffered in the channel into batch and
// posts the result as the final shutdown push. The push is bounded by
// lokiFlushTimeout (not the 10-second per-request timeout) so the goroutine
// cannot outlive the drain deadline.
func (lw *lokiWriter) flushRemaining(batch []lokiEntry) []lokiEntry {
collect:
	for {
		select {
		case entry := <-lw.ch:
			batch = append(batch, entry)
		default:
			break collect
		}
	}
	if len(batch) == 0 {
		return batch[:0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), lokiFlushTimeout)
	defer cancel()
	lw.postBatch(ctx, batch)
	return batch[:0]
}

// extractLevel scans a zerolog JSON log line for the "level" field value using
// a substring scan. It is deliberately independent of json.Unmarshal — not an
// allocation optimization, since streamKeyFromLine unmarshals every line for
// the label fields anyway. Its value is robustness: Write accepts arbitrary
// bytes, and a line that fails a full parse still groups by its extracted level
// instead of collapsing to "unknown". If the field is absent, empty, or the
// value is malformed, "unknown" is returned.
func extractLevel(line string) string {
	const prefix = `"level":"`
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return levelUnknown
	}
	rest := line[idx+len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return levelUnknown
	}
	return rest[:end]
}

// streamKeyFromLine returns the complete Loki stream grouping key for one
// zerolog JSON log line. Level is always promoted (the zerolog level set is
// bounded); the other four low-cardinality keys are promoted only when the
// line's exact (key, value) pair was sanctioned via WithLoggerLabels or
// Logger.WithLabels. Everything else — trace_id, span_id, user_id, durations,
// resource IDs, and payload fields that merely reuse a label key name (e.g.
// .Str("host", req.Host)) — stays payload-only and never becomes a stream
// label (FR-009, Constitution Principle VIII).
//
// The level is whatever extractLevel returns, including the sentinel
// "unknown" for malformed or missing level fields. Preserving "unknown" (rather
// than blanking it) keeps the level label present on every stream so that
// {level="unknown"} queries surface these lines instead of silently dropping
// the label.
func (lw *lokiWriter) streamKeyFromLine(line string) lokiStreamKey {
	key := lokiStreamKey{level: extractLevel(line)}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return key
	}
	lw.labelMu.RLock()
	defer lw.labelMu.RUnlock()
	key.environment = lw.sanctionedField(fields, lokiLabelEnvironment)
	key.application = lw.sanctionedField(fields, lokiLabelApplication)
	key.host = lw.sanctionedField(fields, lokiLabelHost)
	key.version = lw.sanctionedField(fields, lokiLabelVersion)
	return key
}

// sanctionLabels records the non-empty pairs of labels as sanctioned for
// promotion into Loki stream labels. NewLogger calls it with the final
// WithLoggerLabels value (after all options have run, so LoggerOption order
// does not matter) and Logger.WithLabels calls it for derived loggers, so
// promotion always follows the labels the operator actually declared.
func (lw *lokiWriter) sanctionLabels(labels Labels) {
	lw.labelMu.Lock()
	defer lw.labelMu.Unlock()
	if lw.sanctionedLabels == nil {
		lw.sanctionedLabels = make(map[labelPair]struct{})
	}
	add := func(key, value string) {
		if value != "" {
			lw.sanctionedLabels[labelPair{key: key, value: value}] = struct{}{}
		}
	}
	add(lokiLabelEnvironment, labels.Environment)
	add(lokiLabelApplication, labels.Application)
	add(lokiLabelHost, labels.Host)
	add(lokiLabelVersion, labels.Version)
}

// sanctionedField returns the string value of fields[name] only when that exact
// (key, value) pair is sanctioned for label promotion; otherwise "". Callers
// must hold labelMu.
func (lw *lokiWriter) sanctionedField(fields map[string]json.RawMessage, name string) string {
	value := jsonStringField(fields, name)
	if value == "" {
		return ""
	}
	if _, ok := lw.sanctionedLabels[labelPair{key: name, value: value}]; !ok {
		return ""
	}
	return value
}

func jsonStringField(fields map[string]json.RawMessage, name string) string {
	raw, ok := fields[name]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// streamMap returns the Loki stream label map for this key. Only non-empty
// label values are included (FR-009): trace_id, span_id, and all other
// high-cardinality fields must never appear as stream keys.
func (key lokiStreamKey) streamMap() map[string]string {
	m := make(map[string]string)
	if key.environment != "" {
		m[lokiLabelEnvironment] = key.environment
	}
	if key.application != "" {
		m[lokiLabelApplication] = key.application
	}
	if key.host != "" {
		m[lokiLabelHost] = key.host
	}
	if key.version != "" {
		m[lokiLabelVersion] = key.version
	}
	if key.level != "" {
		m[lokiLabelLevel] = key.level
	}
	return m
}

func compareStreamKeys(a, b lokiStreamKey) bool {
	if a.environment != b.environment {
		return a.environment < b.environment
	}
	if a.application != b.application {
		return a.application < b.application
	}
	if a.host != b.host {
		return a.host < b.host
	}
	if a.version != b.version {
		return a.version < b.version
	}
	return a.level < b.level
}

// postBatch groups log entries by their stream key and POSTs them to the Loki
// push endpoint as a single request with one stream per distinct stream label
// set. Each request is bounded by a 10-second per-request context timeout.
// Non-2xx responses and connection failures are reported as bounded diagnostics
// on the logger's configured writer (FR-005); the affected entries are then
// dropped (fail-open) and never affect the CLI exit code. Response bodies are
// always closed.
func (lw *lokiWriter) postBatch(ctx context.Context, batch []lokiEntry) {
	// Group entries by full stream key: one lokiStream per distinct
	// low-cardinality label set in this batch. Each entry keeps its own
	// enqueue-time timestamp so ordering and time-range queries remain accurate.
	byStream := make(map[lokiStreamKey][][2]string)
	for _, entry := range batch {
		key := lw.streamKeyFromLine(entry.line)
		ts := strconv.FormatInt(entry.ts, 10)
		byStream[key] = append(byStream[key], [2]string{ts, entry.line})
	}

	keys := make([]lokiStreamKey, 0, len(byStream))
	for key := range byStream {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareStreamKeys(keys[i], keys[j])
	})

	streams := make([]lokiStream, 0, len(byStream))
	for _, key := range keys {
		streams = append(streams, lokiStream{
			Stream: key.streamMap(),
			Values: byStream[key],
		})
	}

	body := lokiPushBody{Streams: streams}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, lokiPostTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx, http.MethodPost,
		lw.pushURL,
		bytes.NewReader(bodyJSON),
	)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if lw.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+lw.authToken)
	}

	resp, err := lw.client.Do(req)
	if err != nil {
		// Connection failure: fail-open (FR-005) but keep the promised
		// diagnostic. The *url.Error wrapper is stripped so the message cannot
		// echo credentials an operator embedded in AX_LOKI_URL; %q escapes any
		// control characters in the transport error text.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		lw.diagf("ax: loki push failed: %q\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		lw.diagf("ax: loki push returned %d\n", resp.StatusCode)
	}
}

// diagf writes a bounded Loki diagnostic to the logger's configured writer,
// resolving it lazily through errorWriter so LoggerOption order does not
// matter. Diagnostics are best-effort operational messages (FR-005), never log
// payloads, and must stay free of secrets and unsanitized user input.
func (lw *lokiWriter) diagf(format string, args ...any) {
	if lw.errorWriter == nil || *lw.errorWriter == nil {
		return
	}
	fmt.Fprintf(*lw.errorWriter, format, args...)
}

// WithLokiFromEnv returns a LoggerOption that enables direct Loki push when the
// AX_LOKI_URL environment variable is set. It reads AX_LOKI_URL and
// AX_LOKI_AUTH_TOKEN at construction time. If AX_LOKI_URL is empty or
// malformed, the option is a no-op and a warning is written to the logger's
// configured writer. Push is non-blocking; failures are reported as bounded
// diagnostics on the logger's configured writer (fail-open, FR-005) and never
// affect the CLI exit code. The caller must invoke ax.Flush to drain buffered
// entries at shutdown.
//
// Push diagnostics (connection failures, non-2xx responses) resolve the
// configured writer lazily, so LoggerOption order between WithLokiFromEnv and
// WithLoggerWriter does not matter for them. Construction-time URL warnings are
// emitted when the option runs and go to the writer configured at that point.
func WithLokiFromEnv() LoggerOption {
	return func(cfg *loggerConfig) {
		rawURL := os.Getenv("AX_LOKI_URL")
		if rawURL == "" {
			return // no-op: AX_LOKI_URL not set
		}

		parsed, err := url.Parse(rawURL)
		if err != nil {
			fmt.Fprintf(cfg.writer, "ax: AX_LOKI_URL %q is malformed: %v\n", rawURL, err)
			return
		}
		if parsed.Host == "" {
			fmt.Fprintf(cfg.writer,
				"ax: AX_LOKI_URL %q is missing a scheme://host "+
					"(expected e.g. http://localhost:3100)\n", rawURL)
			return
		}

		// Only http(s) is supported; reject other schemes (e.g. ftp) rather than
		// constructing a sink that fails opaquely at request time.
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			fmt.Fprintf(cfg.writer, "ax: AX_LOKI_URL scheme %q is not http(s)\n", parsed.Scheme)
			return
		}

		// Warn when a non-loopback host uses plain HTTP (US3/T027). Continue
		// constructing the writer; the warning is advisory, not fatal.
		if parsed.Scheme == "http" {
			host := parsed.Hostname()
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				fmt.Fprintf(cfg.writer, "ax: AX_LOKI_URL uses insecure http transport\n")
			}
		}

		// JoinPath preserves any base path and collapses duplicate slashes, so a
		// trailing-slash or path-prefixed AX_LOKI_URL still yields a correct
		// endpoint (avoids the "//loki/api/v1/push" double-slash bug).
		pushURL := parsed.JoinPath("loki", "api", "v1", "push").String()

		lw := &lokiWriter{
			pushURL:       pushURL,
			authToken:     os.Getenv("AX_LOKI_AUTH_TOKEN"),
			errorWriter:   &cfg.writer,
			ch:            make(chan lokiEntry, lokiChannelCap),
			flushRequests: make(chan chan struct{}),
			client:        HTTPClient(),
			done:          make(chan struct{}),
		}
		go lw.run(cfg.ctx)
		cfg.additionalSinks = append(cfg.additionalSinks, lw)
	}
}

// Flush performs a best-effort, non-destructive drain of any buffered Loki log
// entries for the given Logger. It blocks until the buffer is empty, the context
// is cancelled, or an internal 2-second deadline elapses — whichever comes
// first. Remaining entries are dropped after the deadline.
//
// The error return is reserved for future sink implementations: the Loki sink's
// drain returns nil on every path (push failures are fail-open diagnostics on
// the configured writer, never returned errors), so Flush currently always
// returns nil. Callers should keep checking it — new sinks may surface drain
// failures — but a failed Loki push must never change the CLI exit code.
//
// Flush is a no-op (returns nil) when:
//   - l has no Loki sink (AX_LOKI_URL was not set)
//   - l is nil
//   - the sink's background goroutine already stopped because its logger context
//     was cancelled
//
// Callers may invoke Flush multiple times; later writes remain deliverable by a
// later Flush call. Callers should invoke Flush in their shutdown path, before
// os.Exit or cobra.Command cleanup, to ensure in-flight log lines reach Loki.
func Flush(ctx context.Context, l Logger) error {
	if l == nil {
		return nil
	}
	f, ok := l.(flusher)
	if !ok {
		return nil
	}
	return f.flush(ctx)
}
