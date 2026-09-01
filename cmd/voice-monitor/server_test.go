package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
