package mlx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestHealthyTrueOn200(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	if !NewClient(srv.URL).Healthy() {
		t.Error("Healthy() = false, want true for a 200")
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
