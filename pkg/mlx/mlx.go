// Package mlx is an HTTP client for the local mlx-engine server
// (../../mlx-engine/), which runs Voxtral STT and Kokoro TTS on-device via
// MLX. Lives at the top level, not nested under pkg/stt, because the
// server it wraps serves TTS as much as STT — it exposes both Transcribe()
// and Speak() rather than splitting TTS off into a package of its own. Not
// to be confused with pkg/stt/realtime, a different, independent client
// that shells out to voxtral/realtime.py directly (used by
// cmd/voice-monitor) — same underlying model family, two different local
// architectures depending on which command you're looking at.
package mlx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"time"

	"local-whisper/pkg/stt"
)

// Client wraps the local mlx-engine HTTP server.
type Client struct {
	ServerURL string
}

// NewClient creates a new mlx-engine HTTP client.
func NewClient(serverURL string) *Client {
	if serverURL == "" {
		serverURL = "http://127.0.0.1:8765"
	}
	return &Client{
		ServerURL: serverURL,
	}
}

// TranscribeResponse is the expected JSON response from the server
type TranscribeResponse struct {
	Text       string  `json:"text"`
	LatencySec float64 `json:"latency_sec"`
}

// Transcribe transcribes audio by sending it to the MLX Voxtral server
func (c *Client) Transcribe(opts stt.Options) (string, error) {
	// Open the audio file
	file, err := os.Open(opts.AudioPath)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Prepare a form that you will submit to that URL.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add the file to the payload
	fw, err := w.CreateFormFile("audio", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err = io.Copy(fw, file); err != nil {
		return "", fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Close the multipart writer to set the terminating boundary
	w.Close()

	// Construct URL with query parameters
	reqURL := c.ServerURL + "/transcribe"
	if opts.Language != "" {
		q := url.Values{}
		q.Set("language", opts.Language)
		reqURL = reqURL + "?" + q.Encode()
	}

	// Create request
	req, err := http.NewRequest("POST", reqURL, &b)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set content type to multipart/form-data
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("server connection failed (is the mlx-engine running?): %w", err)
	}
	defer res.Body.Close()

	// Parse response
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d: %s", res.StatusCode, string(body))
	}

	var response TranscribeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse server response: %w", err)
	}

	stt.WriteOutputIfRequested(opts, response.Text)
	return response.Text, nil
}

// SpeakOptions are the knobs /speak accepts. Voice may be a comma-separated
// pair ("af_heart,af_sky") for Kokoro's voice blending, and Text may carry
// Kokoro's inline pronunciation markup — both are passed through untouched.
type SpeakOptions struct {
	Text  string  `json:"text"`
	Voice string  `json:"voice"`
	Speed float64 `json:"speed"`
}

// Speak synthesizes text with the server's Kokoro TTS and returns the raw
// WAV bytes. Playback is the caller's problem — see internal/speaker, which
// wraps this with the mute/activity-marker/playback-lock protocol that
// scripts/speak.sh established.
func (c *Client) Speak(opts SpeakOptions) ([]byte, error) {
	payload, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to encode speak request: %w", err)
	}

	req, err := http.NewRequest("POST", c.ServerURL+"/speak", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Longer than Transcribe's 30s: a whole article read aloud in one call
	// is a legitimate request, and the server synthesizes synchronously.
	client := &http.Client{Timeout: 300 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server connection failed (is the mlx-engine running?): %w", err)
	}
	defer res.Body.Close()

	audio, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", res.StatusCode, string(audio))
	}

	return audio, nil
}

// healthTimeout matches the 2s curl timeout scripts/speak.sh uses to decide
// whether to talk to the server or fall back to `say`. /health deliberately
// doesn't load models or reset the server's idle timer, so polling it is
// cheap and won't keep an idle engine pinned open.
const healthTimeout = 2 * time.Second

// Healthy reports whether the mlx-engine server is up and finished its TTS
// warmup. False covers both "not running" and "still warming up" — callers
// only need to know whether /speak is worth attempting.
func (c *Client) Healthy() bool {
	client := &http.Client{Timeout: healthTimeout}
	res, err := client.Get(c.ServerURL + "/health")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode == http.StatusOK
}
