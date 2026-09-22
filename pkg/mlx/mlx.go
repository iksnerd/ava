// Package mlx is an HTTP client for the local mlx-engine server
// (../../mlx-engine/), which runs Kokoro TTS on-device via MLX.
//
// It used to speak to that server's STT half too, behind --engine voxtral.
// That was removed after measuring it: on the same 20s sample whisper.cpp
// took 1.26s against Voxtral's 17s warm and 127s cold, for a near-identical
// transcript. One-shot transcription is pkg/stt/whisper's job now.
//
// Not to be confused with pkg/stt/realtime, an independent client that shells
// out to voxtral/realtime.py for cmd/voice-monitor. Voxtral still earns its
// place there: that path is streaming, and pkg/stt/whisper transcribes a
// complete file per subprocess. (whisper.cpp does ship a streaming example;
// it is simply not what this project wraps.)
package mlx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps the local mlx-engine HTTP server.
type Client struct {
	ServerURL string
}

// DefaultServerURL is where mlx-engine binds. Exported because every command
// that takes a --server-url flag has to name this default in its help text,
// and three commands each spelling the port produced three help strings that
// would have lied the moment the default moved. mlx-engine/server.py,
// mlx-engine/Makefile and scripts/mlx-engine-server.sh spell it too — those
// are pinned by TestDefaultServerURLMatchesEngine rather than shared, since
// Go cannot hand a constant to Python or bash.
const DefaultServerURL = "http://127.0.0.1:8765"

// NewClient creates a new mlx-engine HTTP client.
func NewClient(serverURL string) *Client {
	if serverURL == "" {
		serverURL = DefaultServerURL
	}
	return &Client{
		ServerURL: serverURL,
	}
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
