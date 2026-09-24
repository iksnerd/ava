package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
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
  header .status.reconnecting { color: #fc6; }
  /* Loud on purpose: the server only exits on Ctrl-C, so this state means the
     call transcript has stopped, not that the network blinked. */
  header .status.disconnected { color: #fff; background: #a22; font-weight: 600; padding: 2px 8px; border-radius: 4px; }
  header .actions { float: right; }
  header button {
    font: inherit; font-size: 12px; color: #eee; background: #2a2a2a;
    border: 1px solid #444; border-radius: 5px; padding: 3px 9px; cursor: pointer;
  }
  header button:hover { background: #333; }
  header button:disabled { opacity: .5; cursor: default; }
  main { padding: 20px; }
  #transcript { white-space: pre-wrap; font-size: 18px; line-height: 1.6; max-width: 70ch; }
  .hint { color: #888; font-size: 13px; margin-top: 20px; }
  .hint code { color: #aaa; }
</style>
</head>
<body>
<header>
  <span class="actions">
    <button id="copy" type="button">Copy</button>
    <button id="save" type="button">Save</button>
  </span>
  <strong>Voxtral Realtime Monitor</strong>
  &mdash; <span id="status" class="status reconnecting" aria-live="polite">connecting…</span>
</header>
<main>
  <div id="session" class="hint" aria-live="polite">Waiting for the session to start&hellip;</div>
  <div id="transcript" role="log" aria-live="polite" aria-atomic="false"></div>
  <div id="waiting" class="hint">Listening. Nothing transcribed yet &mdash; if this stays empty while people are talking, the capture device is probably reading silence (see docs/monitor.md).</div>
</main>
<script>
  const el = document.getElementById('transcript');
  const status = document.getElementById('status');
  const session = document.getElementById('session');
  const waiting = document.getElementById('waiting');

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

  // Three states, because EventSource retries by itself after any blip: a
  // momentary drop and a server that has exited both fire onerror, and used to
  // look identical. An error shows amber "reconnecting…"; only if onopen has
  // not fired again within RECONNECT_GRACE_MS does it escalate to red, which
  // also marks the tab title so a backgrounded tab still says it.
  const RECONNECT_GRACE_MS = 10000;
  const title = document.title;
  let giveUp = null;
  const setStatus = (text, cls) => { status.textContent = text; status.className = 'status ' + cls; };
  const disconnected = () => {
    giveUp = null;
    setStatus('disconnected — transcript stopped', 'disconnected');
    document.title = '⚠ Disconnected — ' + title;
  };

  const es = new EventSource('/events');
  es.onopen = () => {
    clearTimeout(giveUp);
    giveUp = null;
    setStatus('connected', 'connected');
    document.title = title;
  };
  es.onerror = () => {
    // CLOSED means the browser will not retry (e.g. a non-SSE response), so
    // there is nothing to wait for.
    if (es.readyState === EventSource.CLOSED) {
      clearTimeout(giveUp);
      disconnected();
      return;
    }
    // Retries fire onerror again every few seconds; restarting the timer on
    // each would mean it never escalates.
    if (giveUp !== null || status.classList.contains('disconnected')) return;
    setStatus('reconnecting…', 'reconnecting');
    giveUp = setTimeout(disconnected, RECONNECT_GRACE_MS);
  };

  es.addEventListener('session', (e) => {
    session.textContent = JSON.parse(e.data).text;
  });

  // The server replays the whole transcript on every connect, and EventSource
  // reconnects by itself after any blip. Appending that replay duplicated the
  // entire transcript — twice after one dropped connection, more on a flaky
  // network — while the badge still read "connected". A snapshot replaces.
  es.addEventListener('snapshot', (e) => {
    el.textContent = '';
    const text = JSON.parse(e.data).text;
    append(text);
    if (text) waiting.hidden = true;
    followTail();
  });

  es.onmessage = (e) => {
    const stick = atBottom();
    append(JSON.parse(e.data).text);
    waiting.hidden = true;
    if (stick) followTail();
  };

  // The transcript is also on disk, but the log path is not always where the
  // reader is. Selection survives a delta now, so Cmd+A works — these just make
  // it one click, and the Save name carries the date so two calls do not collide.
  const stamp = () => new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
  const flash = (btn, word) => {
    const had = btn.textContent;
    btn.textContent = word;
    setTimeout(() => { btn.textContent = had; }, 1200);
  };

  document.getElementById('copy').onclick = async (e) => {
    try {
      await navigator.clipboard.writeText(el.textContent);
      flash(e.target, 'Copied');
    } catch {
      flash(e.target, 'Blocked');   // clipboard needs a secure context
    }
  };

  document.getElementById('save').onclick = (e) => {
    const blob = new Blob([el.textContent], {type: 'text/plain'});
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'transcript-' + stamp() + '.txt';
    a.click();
    URL.revokeObjectURL(a.href);
    flash(e.target, 'Saved');
  };
</script>
</body>
</html>`

// newMux builds the HTTP handler serving the live-transcript page ("/") and
// its SSE feed ("/events") off of h.
func newMux(h *hub) http.Handler {
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

		// Session metadata first, so a page that has not heard a word yet can
		// still say what it is listening to and where the log is going.
		if info := h.sessionInfo(); info != "" {
			writeEvent(w, sessionEvent(info))
			flusher.Flush()
		}

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
			case ev, ok := <-ch:
				if !ok {
					return
				}
				writeEvent(w, ev)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	return loopbackOnly(mux)
}

func writeEvent(w io.Writer, ev event) {
	if ev.name != "" {
		fmt.Fprintf(w, "event: %s\n", ev.name)
	}
	fmt.Fprintf(w, "data: %s\n\n", ev.data)
}

// loopbackOnly refuses any request whose Host is not a loopback name.
// Listening on 127.0.0.1 keeps other machines out but not a web page: with
// DNS rebinding, attacker.example resolves to 127.0.0.1 and the browser reads
// the live call transcript as same-origin. The Host header still names
// attacker.example, which is what this checks.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		switch strings.Trim(host, "[]") {
		case "127.0.0.1", "localhost", "::1":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "forbidden: this server only answers to localhost", http.StatusForbidden)
		}
	})
}
