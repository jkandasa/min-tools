package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// memWriter collects the lines a receiver stores.
type memWriter struct {
	lines []string
}

func (m *memWriter) WriteLine(data []byte) error {
	m.lines = append(m.lines, string(data))
	return nil
}

func (m *memWriter) Path() string { return "memory" }

func (m *memWriter) Close() error { return nil }

func post(t *testing.T, receiver *Receiver, body, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/audit", strings.NewReader(body))
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, req)
	return rec
}

func TestStoresSingleEventAsOneLine(t *testing.T) {
	writer := &memWriter{}
	receiver := NewReceiver("audit", "/audit", "", writer)

	rec := post(t, receiver, "{\n  \"api\": {\n    \"name\": \"PutObject\"\n  }\n}", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(writer.lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(writer.lines))
	}
	if writer.lines[0] != `{"api":{"name":"PutObject"}}` {
		t.Errorf("expected compacted json, got %q", writer.lines[0])
	}
}

func TestStoresBatchedEvents(t *testing.T) {
	writer := &memWriter{}
	receiver := NewReceiver("audit", "/audit", "", writer)

	rec := post(t, receiver, `{"seq":1}`+"\n"+`{"seq":2}`+"\n"+`{"seq":3}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(writer.lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(writer.lines))
	}
	if events, _, _ := receiver.Stats(); events != 3 {
		t.Errorf("expected 3 events, got %d", events)
	}
}

func TestEmptyBodyIsTreatedAsProbe(t *testing.T) {
	writer := &memWriter{}
	receiver := NewReceiver("logger", "/logger", "", writer)

	rec := post(t, receiver, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for the endpoint probe, got %d", rec.Code)
	}
	if len(writer.lines) != 0 {
		t.Errorf("probe must not be stored, got %v", writer.lines)
	}
}

func TestAuthToken(t *testing.T) {
	writer := &memWriter{}
	receiver := NewReceiver("audit", "/audit", "s3cr3t", writer)

	tests := []struct {
		name          string
		authorization string
		want          int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "nope", http.StatusUnauthorized},
		{"raw", "s3cr3t", http.StatusOK},
		{"bearer", "Bearer s3cr3t", http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := post(t, receiver, `{"seq":1}`, test.authorization).Code; got != test.want {
				t.Errorf("expected %d, got %d", test.want, got)
			}
		})
	}

	if _, rejected, _ := receiver.Stats(); rejected != 2 {
		t.Errorf("expected 2 rejected requests, got %d", rejected)
	}
}

func TestNonJSONBodyIsKeptOnOneLine(t *testing.T) {
	writer := &memWriter{}
	receiver := NewReceiver("logger", "/logger", "", writer)

	if rec := post(t, receiver, "plain text\nsecond line", ""); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(writer.lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(writer.lines))
	}
	if strings.Contains(writer.lines[0], "\n") {
		t.Errorf("expected a single line, got %q", writer.lines[0])
	}
}

func TestMethodNotAllowed(t *testing.T) {
	receiver := NewReceiver("audit", "/audit", "", &memWriter{})

	rec := httptest.NewRecorder()
	receiver.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/audit", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
