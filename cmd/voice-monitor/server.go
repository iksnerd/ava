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
  /* Sticky: the connection badge is the only signal that the transcript is
     still live, and it used to scroll away about twenty seconds into a call. */
  header { padding: 12px 20px; background: #1b1b1b; border-bottom: 1px solid #333; position: sticky; top: 0; }
  header .status { font-size: 13px; color: #8f8; }
  header .status.disconnected { color: #f88; }
  main { padding: 20px; }
  #transcript { white-space: pre-wrap; font-size: 18px; line-height: 1.6; max-width: 70ch; }
  .hint { color: #888; font-size: 13px; margin-top: 20px; }
  .hint code { color: #aaa; }
</style>
</head>
<body>
<header>
  <strong>Voxtral Realtime Monitor</strong>
  &mdash; <span id="status" class="status disconnected" aria-live="polite">connecting…</span>
</header>
<main>
  <div id="transcript" role="log" aria-live="polite" aria-atomic="false"></div>
  <div class="hint">Live transcript. Also appended to the log file this was started with &mdash; it is not deleted automatically.</div>
</main>
<script>
  const el = document.getElementById('transcript');
  const status = document.getElementById('status');

  // Append a text node rather than reassigning textContent. Reassigning
  // rebuilt the whole node on every delta, which collapsed any selection the
  // reader had made — so the transcript could not be copied at all while
  // anyone was still talking — and made rendering O(n^2) in transcript length.
  const append = (text) => el.appendChild(document.createTextNode(text));

  // Only follow the tail if the reader is already there. Scrolling
  // unconditionally meant you could not read back a sentence mid-call: the page
  // yanked you to the bottom on the next word.
  const NEAR_BOTTOM_PX = 60;
  const atBottom = () =>
    window.innerHeight + window.scrollY >= document.body.scrollHeight - NEAR_BOTTOM_PX;
  const followTail = () => window.scrollTo(0, document.body.scrollHeight);

  const es = new EventSource('/events');
  es.onopen = () => { status.textContent = 'connected'; status.className = 'status'; };
  es.onerror = () => { status.textContent = 'disconnected'; status.className = 'status disconnected'; };

  // The server replays the whole transcript on every connect, and EventSource
  // reconnects by itself after any blip. Appending that replay duplicated the
  // entire transcript — twice after one dropped connection, more on a flaky
  // network — while the badge still read "connected". A snapshot replaces.
  es.addEventListener('snapshot', (e) => {
    el.textContent = '';
    append(JSON.parse(e.data).text);
    followTail();
  });

  es.onmessage = (e) => {
    const stick = atBottom();
    append(JSON.parse(e.data).text);
    if (stick) followTail();
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
			// Named event, not a plain message: EventSource reconnects on its
			// own after any blip and gets this replay again, so the client has
			// to be able to tell "here is the whole transcript" from "here is
			// one more line" and replace rather than append. Without the name
			// a single dropped connection duplicated the entire transcript.
			fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", payload)
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
