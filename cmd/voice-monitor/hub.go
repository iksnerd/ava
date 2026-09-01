package main

import (
	"encoding/json"
	"sync"
	"unicode/utf8"
)

// maxHistoryBytes bounds the in-memory transcript kept for newly opened
// tabs — well beyond any realistic single session, but without a cap it
// grows for as long as voice-monitor is left running unattended.
const maxHistoryBytes = 1 << 20 // 1 MiB

// hub fans transcript deltas out to any connected /events (SSE) clients and
// keeps the full transcript so far so a newly opened tab sees history.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
	history []byte
}

func newHub() *hub {
	return &hub{clients: make(map[chan []byte]bool)}
}

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()
	return ch
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

func (h *hub) snapshot() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.history)
}
