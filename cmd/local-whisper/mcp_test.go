package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iksnerd/local-whisper/internal/speaker"
	"github.com/iksnerd/local-whisper/pkg/stt"
)

// spy records what the MCP tools asked the voice stack to do, standing in
// for internal/speaker and the STT clients so the tools can be exercised
// without audio hardware or a running mlx-engine.
type spy struct {
	spoken     []string
	speakOpts  []speaker.Options
	speakErr   error
	stops      int
	transcribe func(model string, opts stt.Options) (string, error)
}

func (s *spy) deps() mcpDeps {
	return mcpDeps{
		Speak: func(text string, opts speaker.Options) error {
			s.spoken = append(s.spoken, text)
			s.speakOpts = append(s.speakOpts, opts)
			return s.speakErr
		},
		StopSpeaking: func() { s.stops++ },
		Muted:        func() bool { return false },
		Transcribe: func(model string, opts stt.Options) (string, error) {
			if s.transcribe == nil {
				return "", errors.New("no transcriber configured")
			}
			return s.transcribe(model, opts)
		},
	}
}

// connect wires a real MCP client to the server over the SDK's in-memory
// transport, so these tests go through actual protocol plumbing (schema
// validation included) rather than calling handlers directly.
func connect(t *testing.T, deps mcpDeps) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := newMcpServer(deps).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(text.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func TestMcpServerExposesTheVoiceTools(t *testing.T) {
	session := connect(t, (&spy{}).deps())

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
	}

	for _, want := range []string{"speak", "stop_speaking", "list_voices", "transcribe", "speak_accessibility_tree"} {
		if !got[want] {
			t.Errorf("tool %q missing; got %v", want, got)
		}
	}
}

func TestSpeakToolPassesTextVoiceAndSpeed(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hello there", "voice": "bf_emma", "speed": 1.4})
	if res.IsError {
		t.Fatalf("speak returned an error: %s", resultText(t, res))
	}

	if len(s.spoken) != 1 || s.spoken[0] != "hello there" {
		t.Fatalf("spoken = %v, want [hello there]", s.spoken)
	}
	if s.speakOpts[0].Voice != "bf_emma" || s.speakOpts[0].Speed != 1.4 {
		t.Errorf("opts = %+v, want voice bf_emma at speed 1.4", s.speakOpts[0])
	}
}

func TestSpeakToolReportsFailureToTheModel(t *testing.T) {
	s := &spy{speakErr: errors.New("mlx-engine is not running")}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak", map[string]any{"text": "hi"})
	if !res.IsError {
		t.Fatal("want IsError so the model can see the failure and self-correct")
	}
	if !strings.Contains(resultText(t, res), "mlx-engine is not running") {
		t.Errorf("result = %q, want the underlying failure", resultText(t, res))
	}
}

func TestStopSpeakingTool(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	if res := callTool(t, session, "stop_speaking", nil); res.IsError {
		t.Fatalf("stop_speaking returned an error: %s", resultText(t, res))
	}
	if s.stops != 1 {
		t.Errorf("stops = %d, want 1", s.stops)
	}
}

func TestListVoicesToolDescribesVoices(t *testing.T) {
	session := connect(t, (&spy{}).deps())

	text := resultText(t, callTool(t, session, "list_voices", nil))
	for _, want := range []string{"af_heart", "American English, female", "bm_george", "British English, male"} {
		if !strings.Contains(text, want) {
			t.Errorf("list_voices output missing %q; got:\n%s", want, text)
		}
	}
}

func TestTranscribeToolReturnsTheText(t *testing.T) {
	s := &spy{transcribe: func(model string, opts stt.Options) (string, error) {
		if opts.AudioPath != "/tmp/clip.wav" || opts.Language != "es" || opts.BeamSize != 2 {
			t.Errorf("opts = %+v, want the requested path, language and beam size", opts)
		}
		return "hola mundo", nil
	}}
	session := connect(t, s.deps())

	res := callTool(t, session, "transcribe", map[string]any{
		"audio_path": "/tmp/clip.wav",
		"language":   "es",
		"beam_size":  2,
	})
	if res.IsError {
		t.Fatalf("transcribe returned an error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "hola mundo") {
		t.Errorf("result = %q, want the transcript", resultText(t, res))
	}
}

const snapshotFixture = `uid=1_0 RootWebArea "Acme"
  uid=1_1 heading "Overview" level="1"
  uid=1_2 link "Home" focusable
  uid=1_3 button ""
`

func TestSpeakAccessibilityTreeSpeaksAndReportsFindings(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak_accessibility_tree", map[string]any{"snapshot": snapshotFixture})
	if res.IsError {
		t.Fatalf("returned an error: %s", resultText(t, res))
	}

	text := resultText(t, res)
	for _, want := range []string{"heading level 1, Overview", "link, Home", "button with no accessible name"} {
		if !strings.Contains(text, want) {
			t.Errorf("result missing %q; got:\n%s", want, text)
		}
	}

	if len(s.spoken) != 1 {
		t.Fatalf("spoke %d times, want 1 utterance batch", len(s.spoken))
	}
	if !strings.Contains(s.spoken[0], "link, Home") {
		t.Errorf("spoke %q, want the rendered announcements", s.spoken[0])
	}
}

// The same rendering has to be available as a pure formatter: an agent
// auditing a page in a quiet room still needs the utterance list.
func TestSpeakAccessibilityTreeCanSkipSpeaking(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	res := callTool(t, session, "speak_accessibility_tree", map[string]any{
		"snapshot": snapshotFixture,
		"speak":    false,
	})
	if res.IsError {
		t.Fatalf("returned an error: %s", resultText(t, res))
	}
	if len(s.spoken) != 0 {
		t.Errorf("spoke %v, want silence when speak=false", s.spoken)
	}
	if !strings.Contains(resultText(t, res), "link, Home") {
		t.Errorf("result = %q, want the utterances even when not speaking", resultText(t, res))
	}
}

func TestSpeakAccessibilityTreeHonoursMode(t *testing.T) {
	s := &spy{}
	session := connect(t, s.deps())

	text := resultText(t, callTool(t, session, "speak_accessibility_tree", map[string]any{
		"snapshot": snapshotFixture,
		"mode":     "links",
		"speak":    false,
	}))

	if !strings.Contains(text, "link, Home") {
		t.Errorf("result = %q, want the link", text)
	}
	if strings.Contains(text, "heading level 1") {
		t.Errorf("result = %q, want links mode to leave headings out", text)
	}
}

func TestSpeakAccessibilityTreeRejectsAnEmptyTree(t *testing.T) {
	session := connect(t, (&spy{}).deps())

	res := callTool(t, session, "speak_accessibility_tree", map[string]any{"snapshot": "nothing parseable here"})
	if !res.IsError {
		t.Fatal("want an error the model can act on, not a silent empty result")
	}
	if !strings.Contains(resultText(t, res), "take_snapshot") {
		t.Errorf("result = %q, want it to name the tool that produces a snapshot", resultText(t, res))
	}
}

// A muted machine is not a failure, but reporting "spoke 20 characters"
// when nothing came out sends the model off believing the user heard it.
func TestSpeakToolSaysWhenMuted(t *testing.T) {
	s := &spy{}
	deps := s.deps()
	deps.Muted = func() bool { return true }
	session := connect(t, deps)

	res := callTool(t, session, "speak", map[string]any{"text": "into the void"})
	if res.IsError {
		t.Fatalf("muted is not an error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "muted") {
		t.Errorf("result = %q, want it to say the output was muted", resultText(t, res))
	}
}

func TestSpeakAccessibilityTreeSaysWhenMuted(t *testing.T) {
	s := &spy{}
	deps := s.deps()
	deps.Muted = func() bool { return true }
	session := connect(t, deps)

	res := callTool(t, session, "speak_accessibility_tree", map[string]any{"snapshot": snapshotFixture})
	if !strings.Contains(resultText(t, res), "muted") {
		t.Errorf("result = %q, want the mute noted alongside the announcements", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "link, Home") {
		t.Errorf("result = %q, want the announcements regardless", resultText(t, res))
	}
}
