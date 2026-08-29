// Package mlxengine is an HTTP client for the local mlx-engine server
// (../../mlx-engine/), which runs Voxtral STT and Kokoro TTS on-device via
// MLX. Not to be confused with pkg/voxtral, a different, independent client
// that shells out to the voxtral/ Python primitives directly (used by
// cmd/voice-monitor) — same underlying model family, two different local
// architectures depending on which command you're looking at.
package mlxengine

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

// TranscribeOptions contains transcription parameters
type TranscribeOptions struct {
	AudioPath     string
	OutputPath    string
	ContextPrompt string
	Language      string
}

// TranscribeResponse is the expected JSON response from the server
type TranscribeResponse struct {
	Text       string  `json:"text"`
	LatencySec float64 `json:"latency_sec"`
}

// Transcribe transcribes audio by sending it to the MLX Voxtral server
func (c *Client) Transcribe(opts TranscribeOptions) (string, error) {
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

	// Write the output to OutputPath for debugging/caching if requested
	if opts.OutputPath != "" {
		_ = os.WriteFile(opts.OutputPath, []byte(response.Text), 0644)
	}

	return response.Text, nil
}
