package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

// The history replay must be a NAMED SSE event, not a plain message.
// EventSource reconnects by itself after any network blip and receives the
// replay again; a client that cannot distinguish it from a delta appends it,
// duplicating the whole transcript — twice after one drop, more on a flaky
// connection, with the status badge still reading "connected".
func TestNewMuxEventsNamesTheSnapshotEventSoAReconnectCanReplace(t *testing.T) {
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

	// Read up to the first blank line: one SSE frame.
	scanner := bufio.NewScanner(res.Body)
	var frame []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		frame = append(frame, line)
	}

	var sawEventName bool
	for _, line := range frame {
		if line == "event: snapshot" {
			sawEventName = true
		}
	}
	if !sawEventName {
		t.Errorf("first frame = %q, want it to carry `event: snapshot` so a reconnecting client replaces instead of appending", frame)
	}
}

// A delta must stay an unnamed message, or the client's onmessage handler stops
// firing and the page goes silent while the badge still says "connected".
//
// Seeding history and reading the snapshot frame first is what makes this
// deterministic. http.Client.Do returns as soon as the handler flushes headers,
// which happens BEFORE it subscribes — so broadcasting straight after Do() can
// land in history and come back as a snapshot rather than a delta. Consuming
// the snapshot proves the handler is past subscribeWithSnapshot. (CI caught
// this; the race was in the test, not the server.)
func TestNewMuxEventsSendsDeltasAsUnnamedMessages(t *testing.T) {
	h := newHub()
	h.broadcast("history")

	srv := httptest.NewServer(newMux(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer res.Body.Close()

	reader := bufio.NewReader(res.Body)
	readFrame := func() []string {
		t.Helper()
		var frame []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("stream ended mid-frame: %v", err)
			}
			line = strings.TrimRight(line, "\n")
			if line == "" {
				return frame
			}
			frame = append(frame, line)
		}
	}

	if snapshot := readFrame(); !slices.Contains(snapshot, "event: snapshot") {
		t.Fatalf("first frame = %q, want the snapshot replay", snapshot)
	}

	// The handler is subscribed now, so this can only arrive as a delta.
	h.broadcast("a delta")
	for _, line := range readFrame() {
		if strings.HasPrefix(line, "event:") {
			t.Errorf("delta frame carried %q; deltas must be unnamed so onmessage receives them", line)
		}
	}
}

// Session metadata must arrive on its own named frame and must NOT be part of
// the transcript: it is not something a reader should get when they copy the
// text off the page, and a late joiner still needs it.
func TestNewMuxEventsSendsSessionInfoSeparatelyFromTheTranscript(t *testing.T) {
	h := newHub()
	h.setSession("Listening on BlackHole · engine whisper · logging to /tmp/x.txt")
	h.broadcast("some speech")

	srv := httptest.NewServer(newMux(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer res.Body.Close()

	// Read the first two frames. Checking the frame count before scanning
	// again matters: && evaluates left to right, so a trailing Scan() would
	// block on the idle stream until the context deadline.
	scanner := bufio.NewScanner(res.Body)
	var lines []string
	for blanks := 0; blanks < 2; {
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if line == "" {
			blanks++
			continue
		}
		lines = append(lines, line)
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "event: session") {
		t.Errorf("frames %q: want an `event: session` frame so the page can show what it is listening to", joined)
	}
	if !strings.Contains(joined, "BlackHole") {
		t.Errorf("frames %q: session frame should carry the device", joined)
	}
	if strings.Contains(h.snapshot(), "BlackHole") {
		t.Error("session metadata leaked into the transcript history; it would end up in a copied transcript")
	}
}

func TestSetSessionDoesNotTouchTheTranscript(t *testing.T) {
	h := newHub()
	h.broadcast("real speech")
	h.setSession("Listening on Mic")

	if got := h.snapshot(); got != "real speech" {
		t.Errorf("snapshot = %q, want only the transcript", got)
	}
	if h.sessionInfo() != "Listening on Mic" {
		t.Errorf("sessionInfo = %q", h.sessionInfo())
	}
}

// The page is the only place a reader sees the transcript live; the log file is
// on disk but not always where they are. Selection survives a delta now, so
// Cmd+A works, but a button is one click. Guarding the wiring, since a typo in
// an element id fails silently in the browser.
// A dropped connection that EventSource is retrying and a server that has
// exited used to render the same red badge. Guard the pieces that keep them
// apart: an amber state, a grace timer that onopen clears, and no timer reset
// on the repeated onerror each retry fires.
// The transcript of a call must not be served to the network. ":8766" and
// "0.0.0.0:8766" both listen on every interface.
func TestTranscriptServerListensOnLoopbackOnly(t *testing.T) {
	if got := listenAddr(8766); got != "127.0.0.1:8766" {
		t.Errorf("listenAddr(8766) = %q, want 127.0.0.1:8766", got)
	}
}

func TestIndexPageDistinguishesReconnectingFromDisconnected(t *testing.T) {
	for _, want := range []string{
		".status.reconnecting",
		".status.disconnected",
		"'reconnecting'",
		"'disconnected'",
		"RECONNECT_GRACE_MS = 10000",
		"setTimeout(disconnected, RECONNECT_GRACE_MS)",
		"clearTimeout(giveUp)",
		"if (giveUp !== null",
		"EventSource.CLOSED",
		`getElementById('status')`,
	} {
		if !strings.Contains(indexHTML, want) {
			t.Errorf("index page is missing %q", want)
		}
	}
	// onerror must not jump straight to red, or the three states collapse back
	// into two.
	onerror := indexHTML[strings.Index(indexHTML, "es.onerror"):]
	onerror = onerror[:strings.Index(onerror, "\n  };")]
	if !strings.Contains(onerror, "setStatus('reconnecting") {
		t.Error("es.onerror never shows the reconnecting state")
	}
}

func TestIndexPageOffersCopyAndSave(t *testing.T) {
	for _, want := range []string{
		`id="copy"`,
		`id="save"`,
		"navigator.clipboard.writeText",
		"a.download",
		// Save names the file by timestamp so two calls in a session don't collide.
		"transcript-",
	} {
		if !strings.Contains(indexHTML, want) {
			t.Errorf("index page is missing %q", want)
		}
	}
	// Both handlers must bind to ids that exist, or the click does nothing.
	for _, id := range []string{"copy", "save"} {
		if !strings.Contains(indexHTML, `getElementById('`+id+`')`) {
			t.Errorf("no handler bound for #%s", id)
		}
	}
}

// Binding to 127.0.0.1 keeps other machines out, but not a web page: with DNS
// rebinding, attacker.example resolves to 127.0.0.1 and the browser reads the
// live call transcript as same-origin. The Host header still says
// attacker.example, so only loopback names may be served.
func TestServerRejectsForeignHostHeaders(t *testing.T) {
	mux := newMux(newHub())
	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:8766", http.StatusOK},
		{"localhost:8766", http.StatusOK},
		{"[::1]:8766", http.StatusOK},
		{"attacker.example:8766", http.StatusForbidden},
		{"attacker.example", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = tc.host
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("GET / with Host %q = %d, want %d", tc.host, rec.Code, tc.want)
		}
	}
}

// A tab opened before the model finished loading connected with no session
// yet, and setSession never told it: the page said "Waiting for the session
// to start…" while the transcript streamed in underneath.
func TestNewMuxEventsSendsSessionInfoThatArrivesAfterConnecting(t *testing.T) {
	h := newHub()
	srv := httptest.NewServer(newMux(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer res.Body.Close()

	// The handler subscribes after it has flushed the headers, so wait until it
	// has, or setSession could run first and take the connect-time path.
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		h.mu.Lock()
		n := len(h.clients)
		h.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the /events handler never subscribed")
		}
	}
	h.setSession("Listening on BlackHole · engine whisper")

	scanner := bufio.NewScanner(res.Body)
	var prev string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			if prev != "event: session" || !strings.Contains(line, "BlackHole") {
				t.Fatalf("got %q after %q, want the session as an `event: session` frame", line, prev)
			}
			return
		}
		prev = line
	}
	t.Fatalf("stream ended without the session: %v", scanner.Err())
}
