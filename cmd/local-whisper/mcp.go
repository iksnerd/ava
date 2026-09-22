package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/iksnerd/local-whisper/internal/a11y"
	"github.com/iksnerd/local-whisper/internal/buildinfo"
	"github.com/iksnerd/local-whisper/internal/speaker"
	"github.com/iksnerd/local-whisper/internal/ttscontrol"
	"github.com/iksnerd/local-whisper/internal/voiceconfig"
	"github.com/iksnerd/local-whisper/pkg/mlx"
	"github.com/iksnerd/local-whisper/pkg/stt"
)

// mcpDeps are the MCP tools' side effects, injected so the server can be
// tested over a real client session without audio hardware or a running
// mlx-engine.
type mcpDeps struct {
	Speak        func(text string, opts speaker.Options) error
	StopSpeaking func()
	// Muted reports the global mute switch, so a tool can say that nothing
	// was actually heard instead of reporting a successful speak.
	Muted      func() bool
	Transcribe func(model string, opts stt.Options) (string, error)
}

// newMcpCmd serves the local voice stack over MCP on stdio, so any MCP
// client — Claude Code in another repo, another agent — can use the same
// on-device whisper.cpp and Kokoro engines the CLI and the menu bar app use,
// without this repo's scripts/ directory being on disk.
//
// Nothing here may write to stdout: that's the JSON-RPC channel, and a
// stray Println kills the session. Diagnostics go to stderr.
func newMcpCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve speech, transcription and accessibility-tree narration over MCP (stdio)",
		Long: "Serve the local voice stack to MCP clients over stdio.\n\n" +
			"Register it with Claude Code:\n" +
			"  claude mcp add -s user local-whisper -- local-whisper mcp",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			spk := speaker.New(serverURL)
			deps := mcpDeps{
				Speak:        spk.Speak,
				StopSpeaking: ttscontrol.StopSpeaking,
				Muted:        voiceconfig.Muted,
				Transcribe: func(model string, opts stt.Options) (string, error) {
					transcriber, err := newTranscriber(model)
					if err != nil {
						return "", err
					}
					return transcriber.Transcribe(opts)
				},
			}
			return newMcpServer(deps).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}

	cmd.Flags().StringVar(&serverURL, "server-url", "", fmt.Sprintf("mlx-engine base URL (default %s)", mlx.DefaultServerURL))

	return cmd
}

type speakArgs struct {
	Text  string  `json:"text" jsonschema:"the text to speak"`
	Voice string  `json:"voice,omitempty" jsonschema:"Kokoro voice id (see list_voices); a comma-separated pair blends two voices. Defaults to the user's configured voice."`
	Speed float64 `json:"speed,omitempty" jsonschema:"speech rate multiplier; defaults to the user's configured speed"`
	Async bool    `json:"async,omitempty" jsonschema:"return immediately instead of waiting for playback to finish"`
}

type transcribeArgs struct {
	AudioPath string `json:"audio_path" jsonschema:"path to a 16kHz mono WAV file"`
	Language  string `json:"language,omitempty" jsonschema:"language code such as en or es; defaults to en"`
	Model     string `json:"model,omitempty" jsonschema:"whisper model size: base (default) or tiny"`
}

type announceArgs struct {
	Snapshot string  `json:"snapshot" jsonschema:"the accessibility tree from chrome-devtools MCP's take_snapshot, pasted verbatim"`
	Mode     string  `json:"mode,omitempty" jsonschema:"reading (default), headings, links, landmarks or forms"`
	Speak    *bool   `json:"speak,omitempty" jsonschema:"speak the announcements out loud; defaults to true"`
	Voice    string  `json:"voice,omitempty" jsonschema:"Kokoro voice id to narrate with"`
	Speed    float64 `json:"speed,omitempty" jsonschema:"speech rate multiplier"`
}

type noArgs struct{}

// newMcpServer builds the tool surface. Every tool returns human-readable
// text rather than structured output: the caller is a model deciding what
// to do next, and "what did the page just say" is prose.
func newMcpServer(deps mcpDeps) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "local-whisper", Version: buildinfo.Get()}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "speak",
		Description: "Speak text out loud on this Mac using the local Kokoro TTS server, " +
			"falling back to macOS `say` if that server is down. Respects the user's global " +
			"mute switch — a muted machine succeeds silently.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args speakArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Text) == "" {
			return nil, nil, fmt.Errorf("text is empty — nothing to speak")
		}
		if err := deps.Speak(args.Text, speaker.Options{Voice: args.Voice, Speed: args.Speed, Async: args.Async}); err != nil {
			return nil, nil, err
		}
		if deps.Muted() {
			return textResult("%s", mutedNotice), nil, nil
		}
		return textResult("spoke %d characters", len(args.Text)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "stop_speaking",
		Description: "Cancel any speech currently being synthesized or played, including speech " +
			"started by another session's Claude Code voice hooks. Safe to call when nothing is speaking.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args noArgs) (*mcp.CallToolResult, any, error) {
		deps.StopSpeaking()
		return textResult("stopped any speech in flight"), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_voices",
		Description: "List the Kokoro voices available to `speak`. All of them ship in the " +
			"model already downloaded, so any of them can be used immediately.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args noArgs) (*mcp.CallToolResult, any, error) {
		return textResult("%s", formatVoices(voiceconfig.Load().Voice)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "transcribe",
		Description: "Transcribe a WAV file on this machine to text, on-device, with whisper.cpp. " +
			"Needs no server and works on any Mac.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args transcribeArgs) (*mcp.CallToolResult, any, error) {
		model := args.Model
		if model == "" {
			model = "base"
		}
		if err := validateModel(model); err != nil {
			return nil, nil, err
		}
		language := args.Language
		if language == "" {
			language = "en"
		}

		text, err := deps.Transcribe(model, stt.Options{AudioPath: args.AudioPath, Language: language})
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(text) == "" {
			return textResult("no speech detected in %s", args.AudioPath), nil, nil
		}
		return textResult("%s", text), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "speak_accessibility_tree",
		Description: "Render a Chrome accessibility tree as what a screen reader would announce, " +
			"and speak it. Pass chrome-devtools MCP's take_snapshot output verbatim. " +
			"Modes: reading (whole page in order), headings, links, landmarks, forms. " +
			"Returns the announcements as text plus findings that only show up when a page is heard.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args announceArgs) (*mcp.CallToolResult, any, error) {
		nodes := a11y.Parse(args.Snapshot)
		if len(nodes) == 0 {
			return nil, nil, fmt.Errorf("no accessibility nodes in that input — pass chrome-devtools MCP's take_snapshot output verbatim (lines like `uid=1_0 RootWebArea \"Title\"`)")
		}

		mode := a11y.Mode(args.Mode)
		if args.Mode == "" {
			mode = a11y.ModeReading
		}
		utterances := a11y.Announce(nodes, mode)
		if len(utterances) == 0 {
			return textResult("nothing to announce in %s mode — the page has no matching nodes", mode), nil, nil
		}

		script := strings.Join(utterances, ". ")
		shouldSpeak := args.Speak == nil || *args.Speak
		if shouldSpeak {
			if err := deps.Speak(script, speaker.Options{Voice: args.Voice, Speed: args.Speed}); err != nil {
				return nil, nil, err
			}
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "%d announcements in %s mode:\n\n", len(utterances), mode)
		for _, u := range utterances {
			fmt.Fprintf(&sb, "  %s\n", u)
		}
		if findings := a11y.Findings(nodes); len(findings) > 0 {
			// Page-wide even when mode announced only part of it.
			sb.WriteString("\nFindings (whole page):\n")
			for _, f := range findings {
				fmt.Fprintf(&sb, "  - %s\n", f)
			}
		}
		switch {
		case !shouldSpeak:
			sb.WriteString("\n(not spoken: speak=false)")
		case deps.Muted():
			sb.WriteString("\n(" + mutedNotice + ")")
		}
		return textResult("%s", sb.String()), nil, nil
	})

	return server
}

// mutedNotice is what every speaking tool says instead of claiming success
// the user never heard.
const mutedNotice = "voice output is muted — nothing was played; unmute in the Ava menu bar app"

func textResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}
