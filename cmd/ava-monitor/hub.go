package main

import (
	"encoding/json"
	"sync"
	"unicode/utf8"
)

// maxHistoryBytes bounds the in-memory transcript kept for newly opened
// tabs — well beyond any realistic single session, but without a cap it
// grows for as long as ava-monitor is left running unattended.
const maxHistoryBytes = 1 << 20 // 1 MiB

// hub fans transcript deltas out to any connected /events (SSE) clients and
// keeps the full transcript so far so a newly opened tab sees history.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
	history []byte
	// session describes what this run is listening to. Kept out of history on
	// purpose: it is not transcript, and it should not end up in the text a
	// reader copies off the page.
	session string
}

func newHub() *hub {
	return &hub{clients: make(map[chan []byte]bool)}
}

func (h *hub) subscribe() chan []byte {
	_, ch := h.subscribeWithSnapshot()
	return ch
}

// subscribeWithSnapshot registers a client and reads the transcript so far in
// one critical section. Doing the two separately drops any delta broadcast in
// the gap between them: it is already too late to appear in the snapshot and
// too early to reach the channel. A reader then sits waiting for a line that
// has already been and gone.
func (h *hub) subscribeWithSnapshot() (string, chan []byte) {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = true
	return string(h.history), ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *hub) broadcast(text string) {
	payload, _ := json.Marshal(map[string]string{"text": text})

	h.mu.Lock()
	h.history = append(h.history, text...)
	if len(h.history) > maxHistoryBytes {
		trim := len(h.history) - maxHistoryBytes
		// Advance to the next rune boundary so a trimmed snapshot never
		// starts mid-character (transcripts can be non-ASCII: Bulgarian via
		// the whisper engine, diarization labels, etc.).
		for trim < len(h.history) && !utf8.RuneStart(h.history[trim]) {
			trim++
		}
		kept := make([]byte, len(h.history)-trim)
		copy(kept, h.history[trim:])
		h.history = kept
	}
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
			// Slow client - drop this update rather than block the pipeline.
		}
	}
	h.mu.Unlock()
}

// setSession records what the page should say it is listening to. The values
// already went to stdout and the log file on the "ready" event; the page showed
// only a generic line that did not even name the log file it mentioned.
func (h *hub) setSession(info string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.session = info
}

func (h *hub) sessionInfo() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session
}

func (h *hub) snapshot() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.history)
}
