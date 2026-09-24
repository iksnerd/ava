package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

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

func TestHubHistoryIsBounded(t *testing.T) {
	h := newHub()

	// Multi-byte UTF-8 text (é is 2 bytes) so a naive byte-offset trim would
	// risk starting the kept snapshot mid-character.
	chunk := strings.Repeat("café ", 200) // 1200 bytes/chunk
	const chunks = 1200                   // ~1.4MB total, well past maxHistoryBytes
	for range chunks {
		h.broadcast(chunk)
	}

	snap := h.snapshot()
	if len(snap) > maxHistoryBytes {
		t.Fatalf("snapshot len = %d, want <= %d (maxHistoryBytes)", len(snap), maxHistoryBytes)
	}
	if len(snap) == 0 {
		t.Fatal("snapshot is empty after broadcasting well past the cap")
	}
	if !utf8.ValidString(snap) {
		t.Fatalf("snapshot is not valid UTF-8 after trimming (mid-rune cut): %q", snap[:50])
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
	// A subscriber that never reads its channel: broadcast must disconnect
	// it rather than block, once its buffer (cap 16) fills.
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

// A subscriber whose buffer filled used to have updates silently dropped: the
// tab stayed "connected" with holes in its transcript, and Copy and Save took
// the holes with them. Closing its channel ends the SSE response instead, so
// the browser reconnects and gets a whole snapshot.
func TestHubDisconnectsASubscriberThatFallsBehind(t *testing.T) {
	h := newHub()
	ch := h.subscribe()

	for range 100 {
		h.broadcast("x")
	}

	closed := false
drain:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				closed = true
				break drain
			}
		default:
			break drain
		}
	}
	if !closed {
		t.Fatal("a subscriber that fell behind was kept connected with updates missing")
	}
	h.unsubscribe(ch) // the SSE handler still defers this; it must not panic
}
