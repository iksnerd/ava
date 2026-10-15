package mlx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/protocol"
	"github.com/iksnerd/ava/pkg/stt/captions"
)

func TestNewClientDefaultURL(t *testing.T) {
	c := NewClient("")
	if c.ServerURL != "http://127.0.0.1:8765" {
		t.Errorf("ServerURL = %q, want default", c.ServerURL)
	}
}

func TestNewClientCustomURL(t *testing.T) {
	c := NewClient("http://example.internal:9999")
	if c.ServerURL != "http://example.internal:9999" {
		t.Errorf("ServerURL = %q, want the custom URL", c.ServerURL)
	}
}

type speakRecorder struct {
	lastPath string
	lastBody map[string]any
	respond  func(w http.ResponseWriter)
}

func (h *speakRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastPath = r.URL.Path
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &h.lastBody)
	h.respond(w)
}

func wavResponder(audio []byte) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(audio)
	}
}

func TestSpeakReturnsAudioAndSendsTextVoiceSpeed(t *testing.T) {
	want := []byte("RIFF....WAVEfmt ")
	h := &speakRecorder{respond: wavResponder(want)}
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.Speak(SpeakOptions{Text: "hello there", Voice: "bf_emma", Speed: 1.3})
	if err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("audio = %q, want %q", got, want)
	}

	if h.lastPath != "/speak" {
		t.Errorf("path = %q, want /speak", h.lastPath)
	}
	if h.lastBody["text"] != "hello there" {
		t.Errorf("text = %v, want %q", h.lastBody["text"], "hello there")
	}
	if h.lastBody["voice"] != "bf_emma" {
		t.Errorf("voice = %v, want %q", h.lastBody["voice"], "bf_emma")
	}
	if h.lastBody["speed"] != 1.3 {
		t.Errorf("speed = %v, want 1.3", h.lastBody["speed"])
	}
}

func TestSpeakServerNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("tts not loaded"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Speak(SpeakOptions{Text: "hi"})
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "tts not loaded") {
		t.Errorf("error = %v, want it to mention the status code and body", err)
	}
}

func TestSpeakServerUnreachable(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close()

	c := NewClient(url)
	_, err := c.Speak(SpeakOptions{Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "is the mlx-engine running") {
		t.Errorf("err = %v, want a connection-failure hint", err)
	}
}

// --- Healthy -----------------------------------------------------------

func TestHealthyTrueWhenTheEngineReportsOurProtocol(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`{"status":"ok","protocol":"` + protocol.EngineProtocol + `"}`))
	}))
	defer srv.Close()

	if !NewClient(srv.URL).Healthy() {
		t.Error("Healthy() = false, want true for an engine on our protocol")
	}
	if path != "/health" {
		t.Errorf("path = %q, want /health", path)
	}
}

func TestHealthyFalseWhenUnreachable(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close()

	if NewClient(url).Healthy() {
		t.Error("Healthy() = true, want false when nothing is listening")
	}
}

func TestHealthyFalseOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if NewClient(srv.URL).Healthy() {
		t.Error("Healthy() = true, want false for a 503")
	}
}

func healthServer(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL)
}

// Check tells an engine that speaks this client's protocol from one that is
// up but older, and both from none at all. The middle case used to read as
// healthy, so a stale engine left running across an upgrade was spoken to.
func TestCheckTellsReadyFromStaleFromDown(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   State
	}{
		"matching protocol": {200, `{"status":"ok","protocol":"` + protocol.EngineProtocol + `"}`, Ready},
		"no protocol (old)": {200, `{"status":"ok","tts_loaded":true}`, Stale},
		"another version":   {200, `{"status":"ok","protocol":"ava-engine/0"}`, Stale},
		"not json":          {200, `ok`, Stale},
		"warming up":        {503, `{"status":"warming"}`, Down},
	} {
		t.Run(name, func(t *testing.T) {
			if got := healthServer(t, tc.status, tc.body).Check(); got != tc.want {
				t.Errorf("Check() = %v, want %v", got, tc.want)
			}
		})
	}

	if got := NewClient("http://127.0.0.1:1").Check(); got != Down {
		t.Errorf("Check() with nothing listening = %v, want Down", got)
	}
}

// Healthy stays the question callers ask: is /speak worth attempting. A stale
// engine is not.
func TestHealthyIsFalseForAStaleEngine(t *testing.T) {
	if healthServer(t, 200, `{"status":"ok"}`).Healthy() {
		t.Error("Healthy() = true for an engine that reports no protocol")
	}
}

// --- SpeakTimed ----------------------------------------------------------

func jsonResponder(body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

func TestSpeakTimedDecodesTheAudioAndTheSegments(t *testing.T) {
	// "RIFFwav" base64-encoded.
	h := &speakRecorder{respond: jsonResponder(`{"sample_rate":24000,"audio":"UklGRndhdg==",` +
		`"segments":[{"text":"Once upon a time.","start_ms":0,"end_ms":1500},{"text":"The end.","start_ms":1500,"end_ms":2000}]}`)}
	srv := httptest.NewServer(h)
	defer srv.Close()

	got, err := NewClient(srv.URL).SpeakTimed(SpeakOptions{Text: "x", Voice: "bf_emma", Speed: 0.9, SplitPattern: `(?<=[.!?])\s+`})
	if err != nil {
		t.Fatalf("SpeakTimed() error = %v", err)
	}
	if string(got.Audio) != "RIFFwav" {
		t.Errorf("audio = %q, want RIFFwav", got.Audio)
	}
	want := []captions.Segment{{Text: "Once upon a time.", StartMS: 0, EndMS: 1500}, {Text: "The end.", StartMS: 1500, EndMS: 2000}}
	if len(got.Segments) != 2 || got.Segments[0] != want[0] || got.Segments[1] != want[1] {
		t.Errorf("segments = %+v, want %+v", got.Segments, want)
	}
	if h.lastPath != "/speak/timed" {
		t.Errorf("path = %q, want /speak/timed", h.lastPath)
	}
	if h.lastBody["split_pattern"] != `(?<=[.!?])\s+` || h.lastBody["voice"] != "bf_emma" {
		t.Errorf("body = %v, want the voice and split pattern sent", h.lastBody)
	}
}

// split_pattern is only sent when set, so plain /speak keeps its request body.
func TestSpeakOmitsSplitPatternWhenUnset(t *testing.T) {
	h := &speakRecorder{respond: wavResponder([]byte("wav"))}
	srv := httptest.NewServer(h)
	defer srv.Close()

	if _, err := NewClient(srv.URL).Speak(SpeakOptions{Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, present := h.lastBody["split_pattern"]; present {
		t.Errorf("body = %v, want no split_pattern when unset", h.lastBody)
	}
}

func TestSpeakTimedReportsAServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"out of memory"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).SpeakTimed(SpeakOptions{Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "out of memory") {
		t.Errorf("err = %v, want the status and the reason", err)
	}
}

func TestSpeakTimedRejectsABodyThatIsNotTheContract(t *testing.T) {
	srv := httptest.NewServer(&speakRecorder{respond: jsonResponder(`not json`)})
	defer srv.Close()

	if _, err := NewClient(srv.URL).SpeakTimed(SpeakOptions{Text: "x"}); err == nil {
		t.Error("SpeakTimed accepted a body that is not JSON")
	}
}
