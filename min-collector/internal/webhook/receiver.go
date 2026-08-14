// Package webhook receives the MinIO logger and audit webhooks over HTTP and
// stores every event as newline delimited JSON.
package webhook

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// maxBodySize caps a single webhook request body.
const maxBodySize = 32 << 20 // 32 MiB

// Writer is the subset of rotate.Writer used by a Receiver.
type Writer interface {
	WriteLine(data []byte) error
	Path() string
	Close() error
}

// Receiver handles one webhook stream, that is logger or audit.
type Receiver struct {
	name      string
	path      string
	authToken string
	writer    Writer

	events   atomic.Int64
	pings    atomic.Int64
	rejected atomic.Int64
	errors   atomic.Int64
}

// NewReceiver creates a receiver for the given stream.
func NewReceiver(name, path, authToken string, writer Writer) *Receiver {
	return &Receiver{
		name:      name,
		path:      path,
		authToken: authToken,
		writer:    writer,
	}
}

// Name returns the stream name.
func (r *Receiver) Name() string { return r.name }

// Path returns the HTTP path the receiver is registered on.
func (r *Receiver) Path() string { return r.path }

// File returns the active file the stream is written to.
func (r *Receiver) File() string { return r.writer.Path() }

// Stats returns the event, rejected and error counters.
func (r *Receiver) Stats() (events, rejected, errs int64) {
	return r.events.Load(), r.rejected.Load(), r.errors.Load()
}

// Close flushes and closes the underlying file.
func (r *Receiver) Close() error { return r.writer.Close() }

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// MinIO posts events, other methods are almost always a misconfiguration.
	if req.Method != http.MethodPost && req.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "only POST is accepted", http.StatusMethodNotAllowed)
		return
	}

	if !r.authorized(req) {
		r.rejected.Add(1)
		logx.Warnf("%s: rejected request from %s, authorization token mismatch", r.name, req.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBodySize))
	if err != nil {
		r.errors.Add(1)
		logx.Errorf("%s: failed to read request body: %v", r.name, err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// MinIO probes the endpoint with an empty body when the target is
	// configured, that probe must succeed for the webhook to come up.
	if len(bytes.TrimSpace(body)) == 0 {
		r.pings.Add(1)
		logx.Debugf("%s: received endpoint probe from %s", r.name, req.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		return
	}

	count, err := r.store(body)
	if err != nil {
		r.errors.Add(1)
		logx.Errorf("%s: failed to store events: %v", r.name, err)
		http.Error(w, "failed to store events", http.StatusInternalServerError)
		return
	}

	r.events.Add(int64(count))
	logx.Debugf("%s: stored %d event(s) from %s", r.name, count, req.RemoteAddr)
	w.WriteHeader(http.StatusOK)
}

// store writes the body as newline delimited JSON. MinIO sends a single JSON
// object per request, or several of them back to back when the target is
// configured with a batch size, so the body is decoded as a stream.
func (r *Receiver) store(body []byte) (int, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))

	var (
		compact bytes.Buffer
		count   int
	)

	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Not valid JSON, keep the payload as a single line rather than
			// dropping data that may matter while debugging.
			if count == 0 {
				return 1, r.writer.WriteLine(flatten(body))
			}
			logx.Warnf("%s: trailing data in request body is not valid JSON", r.name)
			break
		}

		compact.Reset()
		if err := json.Compact(&compact, raw); err != nil {
			if err := r.writer.WriteLine(flatten(raw)); err != nil {
				return count, err
			}
		} else if err := r.writer.WriteLine(compact.Bytes()); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// authorized compares the Authorization header with the configured token.
// MinIO sends the value of auth_token verbatim, an explicit "Bearer " prefix
// is accepted as well.
func (r *Receiver) authorized(req *http.Request) bool {
	if r.authToken == "" {
		return true
	}

	got := strings.TrimSpace(req.Header.Get("Authorization"))
	if got == "" {
		return false
	}
	if secureEqual(got, r.authToken) {
		return true
	}

	trimmed := strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
	want := strings.TrimSpace(strings.TrimPrefix(r.authToken, "Bearer "))
	return secureEqual(trimmed, want)
}

func secureEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// flatten turns a payload into a single line so one record stays one line.
func flatten(data []byte) []byte {
	replacer := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")
	return []byte(replacer.Replace(string(bytes.TrimSpace(data))))
}
