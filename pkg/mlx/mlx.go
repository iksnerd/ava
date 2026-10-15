// Package mlx is an HTTP client for the local mlx-engine server
// (../../mlx-engine/), which runs Kokoro TTS on-device via MLX.
//
// It used to speak to that server's STT half too, behind --engine voxtral.
// That was removed after measuring it: on the same 20s sample whisper.cpp
// took 1.26s against Voxtral's 17s warm and 127s cold, for a near-identical
// transcript. One-shot transcription is pkg/stt/whisper's job now.
//
// Not to be confused with pkg/stt/realtime, an independent client that shells
// out to voxtral/realtime.py for internal/monitor. Voxtral still earns its
// place there: that path is streaming, and pkg/stt/whisper transcribes a
// complete file per subprocess. (whisper.cpp does ship a streaming example;
// it is simply not what this project wraps.)
package mlx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/iksnerd/ava/internal/protocol"
	"github.com/iksnerd/ava/pkg/stt/captions"
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
const DefaultServerURL = protocol.EngineURL

// NewClient creates a new mlx-engine HTTP client.
func NewClient(serverURL string) *Client {
	if serverURL == "" {
		serverURL = DefaultServerURL
	}
	return &Client{
		ServerURL: serverURL,
	}
}

// maxResponseBytes caps how much of a response is read into memory. A WAV of
// ten minutes of speech is under 30 MB (a little over 38 MB as /speak/timed's
// base64), so this leaves room for far more than anyone speaks at once, and
// stops a process squatting on the engine's port, or a runaway engine, from
// filling memory. A variable so a test can lower it.
var maxResponseBytes = 256 << 20

// SpeakOptions are the knobs /speak accepts. Voice may be a comma-separated
// pair ("af_heart,af_sky") for Kokoro's voice blending, and Text may carry
// Kokoro's inline pronunciation markup — both are passed through untouched.
type SpeakOptions struct {
	Text  string  `json:"text"`
	Voice string  `json:"voice"`
	Speed float64 `json:"speed"`
	// SplitPattern is Kokoro's chunking regex. Unset keeps the engine's own
	// default, which splits on newlines. It decides how fine SpeakTimed's
	// segments are: a sentence pattern gives one segment per sentence.
	SplitPattern string `json:"split_pattern,omitempty"`
}

// TimedSpeech is the audio of a whole text and the segments that make it up.
type TimedSpeech struct {
	Audio []byte
	// Segments are the chunks of the text with where each sits in the audio,
	// in milliseconds from the start.
	Segments []captions.Segment
}

// Speak synthesizes text with the server's Kokoro TTS and returns the raw
// WAV bytes. Playback is the caller's problem — see internal/speaker, which
// wraps this with the mute/activity-marker/playback-lock protocol that
// scripts/speak.sh established.
func (c *Client) Speak(ctx context.Context, opts SpeakOptions) ([]byte, error) {
	return c.post(ctx, "/speak", opts)
}

// SpeakTimed is Speak with the time each chunk of text occupies, which is
// what captions need. See docs/engine-api.md for the endpoint.
func (c *Client) SpeakTimed(ctx context.Context, opts SpeakOptions) (TimedSpeech, error) {
	body, err := c.post(ctx, "/speak/timed", opts)
	if err != nil {
		return TimedSpeech{}, err
	}
	var resp struct {
		Audio    []byte             `json:"audio"` // base64 in the JSON
		Segments []captions.Segment `json:"segments"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return TimedSpeech{}, fmt.Errorf("unexpected /speak/timed response: %w", err)
	}
	return TimedSpeech{Audio: resp.Audio, Segments: resp.Segments}, nil
}

// post sends opts to a synthesis endpoint and returns the response body. The
// one place a synthesis request is built, so /speak and /speak/timed cannot
// differ in timeout or in how a failure reads.
func (c *Client) post(ctx context.Context, path string, opts SpeakOptions) ([]byte, error) {
	payload, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to encode speak request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ServerURL+path, bytes.NewReader(payload))
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

	body, err := io.ReadAll(io.LimitReader(res.Body, int64(maxResponseBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("server returned status %d with a response over the cap of %d bytes: "+
			"something other than the engine may be answering on %s", res.StatusCode, maxResponseBytes, c.ServerURL)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", res.StatusCode, string(body))
	}

	return body, nil
}

// healthTimeout bounds the check that decides whether to talk to the server
// or fall back to `say`. /health deliberately
// doesn't load models or reset the server's idle timer, so polling it is
// cheap and won't keep an idle engine pinned open.
const healthTimeout = 2 * time.Second

// State is what a /health check found.
type State int

const (
	// Down covers both "not running" and "still warming up": nothing worth
	// sending /speak to yet.
	Down State = iota
	// Stale is an engine that answers but does not speak this client's
	// protocol, typically one left running from before an upgrade. Starting
	// another would fail to bind, and speaking to it would fail in ways that
	// look like it being down, so it is reported as its own case.
	Stale
	// Ready is an engine that speaks this protocol and finished its warmup.
	Ready
)

func (s State) String() string {
	switch s {
	case Stale:
		return "stale"
	case Ready:
		return "ready"
	default:
		return "down"
	}
}

// Check asks /health who is answering. The engine reports its protocol name
// and version there (internal/protocol, engine_protocol); anything else with
// a 200, including an older engine that reports none, is Stale.
func (c *Client) Check(ctx context.Context) State {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ServerURL+"/health", nil)
	if err != nil {
		return Down
	}
	client := &http.Client{Timeout: healthTimeout}
	res, err := client.Do(req)
	if err != nil {
		return Down
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return Down
	}
	var h struct {
		Protocol string `json:"protocol"`
	}
	if json.Unmarshal(body, &h) != nil || h.Protocol != protocol.EngineProtocol {
		return Stale
	}
	return Ready
}

// Healthy reports whether /speak is worth attempting: an engine that speaks
// this protocol and finished its TTS warmup.
func (c *Client) Healthy(ctx context.Context) bool {
	return c.Check(ctx) == Ready
}
