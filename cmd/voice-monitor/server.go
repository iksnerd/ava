package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const indexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Voxtral Realtime Monitor</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #111; color: #eee; }
  header { padding: 12px 20px; background: #1b1b1b; border-bottom: 1px solid #333; }
  header .status { font-size: 13px; color: #8f8; }
  header .status.disconnected { color: #f88; }
  main { padding: 20px; }
  #transcript { white-space: pre-wrap; font-size: 18px; line-height: 1.6; max-width: 800px; }
  .hint { color: #888; font-size: 13px; margin-top: 20px; }
</style>
</head>
<body>
<header>
  <strong>Voxtral Realtime Monitor</strong>
  &mdash; <span id="status" class="status disconnected">connecting…</span>
</header>
<main>
  <div id="transcript"></div>
  <div class="hint">Live transcript from Voxtral Mini 4B Realtime. Also being written to the transcript log file on disk.</div>
</main>
<script>
  const el = document.getElementById('transcript');
  const status = document.getElementById('status');
  const es = new EventSource('/events');
  es.onopen = () => { status.textContent = 'connected'; status.className = 'status'; };
  es.onerror = () => { status.textContent = 'disconnected'; status.className = 'status disconnected'; };
  es.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    el.textContent += msg.text;
    window.scrollTo(0, document.body.scrollHeight);
  };
</script>
</body>
</html>`

// newMux builds the HTTP handler serving the live-transcript page ("/") and
// its SSE feed ("/events") off of h.
func newMux(h *hub) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush() // send headers now so the browser's EventSource fires onopen immediately, not on the first delta

		// Snapshot and subscribe together — see subscribeWithSnapshot. Taking
		// the snapshot first and subscribing after loses anything broadcast in
		// between.
		snap, ch := h.subscribeWithSnapshot()
		defer h.unsubscribe(ch)

		if snap != "" {
			payload, _ := json.Marshal(map[string]string{"text": snap})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		for {
			select {
			case payload, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	return mux
}
