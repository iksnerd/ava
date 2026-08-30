package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOrDefault(t *testing.T) {
	cases := []struct{ s, def, want string }{
		{"", "fallback", "fallback"},
		{"value", "fallback", "value"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := orDefault(tc.s, tc.def); got != tc.want {
			t.Errorf("orDefault(%q, %q) = %q, want %q", tc.s, tc.def, got, tc.want)
		}
	}
}

func TestLanguageSuffix(t *testing.T) {
	cases := []struct{ language, want string }{
		{"", ""},
		{"en", ""},
		{"bg", ", language: bg"},
		{"es", ", language: es"},
	}
	for _, tc := range cases {
		if got := languageSuffix(tc.language); got != tc.want {
			t.Errorf("languageSuffix(%q) = %q, want %q", tc.language, got, tc.want)
		}
	}
}

func TestResolvePythonPath(t *testing.T) {
	cases := []struct {
		pythonPath, voxtralDir, want string
	}{
		{"", "voxtral", "voxtral/.venv/bin/python3"},
		{"/custom/python3", "voxtral", "/custom/python3"},
		{"", "/abs/voxtral", "/abs/voxtral/.venv/bin/python3"},
	}
	for _, tc := range cases {
		if got := resolvePythonPath(tc.pythonPath, tc.voxtralDir); got != tc.want {
			t.Errorf("resolvePythonPath(%q, %q) = %q, want %q", tc.pythonPath, tc.voxtralDir, got, tc.want)
		}
	}
}

func TestDefaultLogPath(t *testing.T) {
	fixedTime := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)
	got := defaultLogPath("/tmp/voice-input", fixedTime)
	want := "/tmp/voice-input/transcript-20260305-143000.txt"
	if got != want {
		t.Errorf("defaultLogPath() = %q, want %q", got, want)
	}
}

func TestHubBroadcastAndSnapshot(t *testing.T) {
	h := newHub()

	if snap := h.snapshot(); snap != "" {
		t.Fatalf("snapshot of a fresh hub = %q, want empty", snap)
	}

	ch := h.subscribe()
	h.broadcast("hello ")
	h.broadcast("world")

	for _, want := range []string{"hello ", "world"} {
		select {
		case payload := <-ch:
			var msg map[string]string
			if err := json.Unmarshal(payload, &msg); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if msg["text"] != want {
				t.Errorf("delta text = %q, want %q", msg["text"], want)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for broadcast delta")
		}
	}

	if snap := h.snapshot(); snap != "hello world" {
		t.Errorf("snapshot = %q, want %q", snap, "hello world")
	}
}

func TestHubMultipleSubscribers(t *testing.T) {
	h := newHub()
	ch1 := h.subscribe()
	ch2 := h.subscribe()

	h.broadcast("delta")

	for i, ch := range []chan []byte{ch1, ch2} {
		select {
		case payload := <-ch:
			var msg map[string]string
			json.Unmarshal(payload, &msg)
			if msg["text"] != "delta" {
				t.Errorf("subscriber %d got %q, want %q", i, msg["text"], "delta")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for broadcast", i)
		}
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	h := newHub()
	ch := h.subscribe()

	h.unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed channel to return")
	}
}

func TestHubSlowSubscriberDoesNotBlockBroadcast(t *testing.T) {
	h := newHub()
	// A subscriber that never reads its channel: broadcast must drop
	// updates for it rather than block, once its buffer (cap 16) fills.
	_ = h.subscribe()

	done := make(chan struct{})
	go func() {
		for range 100 {
			h.broadcast("x")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked on a slow subscriber instead of dropping updates")
	}
}

func TestHubConcurrentAccess(t *testing.T) {
	h := newHub()
	var wg sync.WaitGroup

	for range 10 {
		wg.Go(func() {
			ch := h.subscribe()
			defer h.unsubscribe(ch)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				case <-time.After(200 * time.Millisecond):
					return
				}
			}
		})
	}

	for range 50 {
		wg.Go(func() {
			h.broadcast("x")
		})
	}

	wg.Wait()
}

func TestNewMuxServesIndexPage(t *testing.T) {
	h := newHub()
	srv := httptest.NewServer(newMux(h))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestNewMuxEventsStreamsSnapshotThenDeltas(t *testing.T) {
	h := newHub()
	h.broadcast("existing history")

	srv := httptest.NewServer(newMux(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(res.Body)
	readEvent := func() string {
		t.Helper()
		for scanner.Scan() {
			line := scanner.Text()
			if payload, ok := strings.CutPrefix(line, "data: "); ok {
				var msg map[string]string
				if err := json.Unmarshal([]byte(payload), &msg); err != nil {
					t.Fatalf("unmarshal SSE payload %q: %v", payload, err)
				}
				return msg["text"]
			}
		}
		t.Fatalf("scanner stopped before an event arrived: %v", scanner.Err())
		return ""
	}

	if got := readEvent(); got != "existing history" {
		t.Errorf("first SSE event = %q, want the pre-existing snapshot %q", got, "existing history")
	}

	h.broadcast("new delta")
	if got := readEvent(); got != "new delta" {
		t.Errorf("second SSE event = %q, want %q", got, "new delta")
	}
}
