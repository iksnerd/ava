package mlx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local-whisper/pkg/stt"
)

const fixtureAudioPath = "testdata/audio.wav"

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

// recordingHandler captures the last request it served so tests can assert
// on how Transcribe built it (method, path, multipart field, query params).
type recordingHandler struct {
	lastMethod      string
	lastPath        string
	lastQuery       url.Values
	lastFileField   string
	lastFileContent []byte
	respond         func(w http.ResponseWriter)
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastMethod = r.Method
	h.lastPath = r.URL.Path
	h.lastQuery = r.URL.Query()

	if err := r.ParseMultipartForm(1 << 20); err == nil {
		for field, headers := range r.MultipartForm.File {
			if len(headers) == 0 {
				continue
			}
			h.lastFileField = field
			f, err := headers[0].Open()
			if err == nil {
				h.lastFileContent, _ = io.ReadAll(f)
				f.Close()
			}
		}
	}

	h.respond(w)
}

func jsonResponder(resp TranscribeResponse) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func TestTranscribeSuccess(t *testing.T) {
	h := &recordingHandler{respond: jsonResponder(TranscribeResponse{Text: "hello world", LatencySec: 0.42})}
	srv := httptest.NewServer(h)
	defer srv.Close()

	outputPath := filepath.Join(t.TempDir(), "out.txt")
	c := NewClient(srv.URL)

	text, err := c.Transcribe(stt.Options{
		AudioPath:  fixtureAudioPath,
		OutputPath: outputPath,
		Language:   "en",
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}

	if h.lastMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", h.lastMethod)
	}
	if h.lastPath != "/transcribe" {
		t.Errorf("path = %q, want /transcribe", h.lastPath)
	}
	if got := h.lastQuery.Get("language"); got != "en" {
		t.Errorf("language query param = %q, want %q", got, "en")
	}
	if h.lastFileField != "audio" {
		t.Errorf("multipart file field = %q, want %q", h.lastFileField, "audio")
	}

	wantAudio, err := os.ReadFile(fixtureAudioPath)
	if err != nil {
		t.Fatalf("read fixture audio: %v", err)
	}
	if string(h.lastFileContent) != string(wantAudio) {
		t.Errorf("uploaded audio content didn't match the fixture file")
	}

	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(written) != "hello world" {
		t.Errorf("output file content = %q, want %q", written, "hello world")
	}
}

func TestTranscribeOmitsLanguageQueryParamWhenEmpty(t *testing.T) {
	h := &recordingHandler{respond: jsonResponder(TranscribeResponse{Text: "ok"})}
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath}); err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}

	if h.lastQuery.Has("language") {
		t.Errorf("expected no language query param, got %v", h.lastQuery)
	}
}

func TestTranscribeServerNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath})
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("error = %v, want it to mention the status code and body", err)
	}
}

func TestTranscribeMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath})
	if err == nil || !strings.Contains(err.Error(), "failed to parse server response") {
		t.Errorf("err = %v, want a JSON parse error", err)
	}
}

func TestTranscribeServerUnreachable(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close() // nothing listens here anymore

	c := NewClient(url)
	_, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath})
	if err == nil || !strings.Contains(err.Error(), "is the mlx-engine running") {
		t.Errorf("err = %v, want a connection-failure hint", err)
	}
}

func TestTranscribeMissingAudioFile(t *testing.T) {
	c := NewClient("http://127.0.0.1:0")
	_, err := c.Transcribe(stt.Options{AudioPath: filepath.Join(t.TempDir(), "missing.wav")})
	if err == nil || !strings.Contains(err.Error(), "failed to open audio file") {
		t.Errorf("err = %v, want a file-open error", err)
	}
}

// A failure writing OutputPath is documented as non-fatal: the transcription
// itself already succeeded, so Transcribe should still return the text.
func TestTranscribeOutputWriteFailureIsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponder(TranscribeResponse{Text: "still returned"})(w)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	// A path under a nonexistent directory: os.WriteFile can't create it.
	badOutputPath := filepath.Join(t.TempDir(), "no-such-dir", "out.txt")

	text, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath, OutputPath: badOutputPath})
	if err != nil {
		t.Fatalf("Transcribe() error = %v, want nil despite the bad OutputPath", err)
	}
	if text != "still returned" {
		t.Errorf("text = %q, want %q", text, "still returned")
	}
}

func TestTranscribeNoOutputPathSkipsWrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponder(TranscribeResponse{Text: "no file written"})(w)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	text, err := c.Transcribe(stt.Options{AudioPath: fixtureAudioPath})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "no file written" {
		t.Errorf("text = %q, want %q", text, "no file written")
	}
}
