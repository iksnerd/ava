package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"local-whisper/internal/procutil"
	"local-whisper/pkg/stt/realtime"
)

// rootOptions holds the persistent --python/--voxtral-dir flags, shared by
// the root command and the devices subcommand since both need a
// realtime.Client to talk to voxtral/realtime.py.
type rootOptions struct {
	pythonPath string
	voxtralDir string
}

func (o rootOptions) client() *realtime.Client {
	return realtime.NewClient(resolvePythonPath(o.pythonPath, o.voxtralDir), o.voxtralDir)
}

// watchOptions holds the root command's own --flag values (the live-watch
// session), on top of the shared rootOptions.
type watchOptions struct {
	rootOptions
	device     string
	engine     string
	sttModel   string
	language   string
	diarize    bool
	highpassHz float64
	port       int
	logPath    string
}

func newRootCmd() *cobra.Command {
	var opts watchOptions

	cmd := &cobra.Command{
		Use:           "voice-monitor",
		Short:         "Serve a live realtime transcript (mic or loopback device) over SSE, logged to a file",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return watch(opts)
		},
	}

	cmd.PersistentFlags().StringVar(&opts.pythonPath, "python", "", "Path to the Voxtral venv python3 (default: <voxtral-dir>/.venv/bin/python3)")
	cmd.PersistentFlags().StringVar(&opts.voxtralDir, "voxtral-dir", "voxtral", "Path to the voxtral/ primitives directory")

	flags := cmd.Flags()
	flags.StringVar(&opts.device, "device", "", "Input device name substring (e.g. BlackHole) or index. Default: system mic.")
	flags.StringVar(&opts.engine, "engine", "", "STT engine: 'voxtral' (default, <500ms, 13 languages) or 'whisper' (multilingual incl. Bulgarian, ~1s latency)")
	flags.StringVar(&opts.sttModel, "stt-model", "", "Override the engine's default HF repo id")
	flags.StringVar(&opts.language, "language", "", "Language code, e.g. 'bg' for Bulgarian (--engine whisper only)")
	flags.BoolVar(&opts.diarize, "diarize", false, "Tag transcript with speaker labels (--engine whisper only; remote participants only, never your own mic)")
	flags.Float64Var(&opts.highpassHz, "highpass-hz", 0, "High-pass filter cutoff in Hz before STT/diarization (0 = use realtime.py's default of 80Hz)")
	flags.IntVar(&opts.port, "port", 8765, "Local HTTP port to serve the live transcript on")
	flags.StringVar(&opts.logPath, "log", "", "Path to write the transcript log (default: /tmp/voice-input/transcript-<timestamp>.txt)")

	cmd.AddCommand(newDevicesCmd(&opts.rootOptions))

	return cmd
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

// watch is the root command's RunE body: stream realtime deltas to the log
// file and to any browser tabs connected over SSE.
func watch(opts watchOptions) error {
	client := opts.client()

	resolvedLog := opts.logPath
	if resolvedLog == "" {
		if err := os.MkdirAll("/tmp/voice-input", 0755); err != nil {
			return fmt.Errorf("failed to create /tmp/voice-input: %w", err)
		}
		resolvedLog = defaultLogPath("/tmp/voice-input", time.Now())
	}

	logFile, err := os.OpenFile(resolvedLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	h := newHub()

	fmt.Println("🎧 Starting Voxtral Realtime monitor...")
	fmt.Printf("📝 Logging transcript to: %s\n", resolvedLog)

	realtimeOpts := realtime.RealtimeOptions{
		Device:     opts.device,
		Engine:     opts.engine,
		Model:      opts.sttModel,
		Language:   opts.language,
		Diarize:    opts.diarize,
		HighpassHz: opts.highpassHz,
	}

	var lastSpeaker string

	stop, err := client.StreamRealtime(realtimeOpts, func(delta realtime.RealtimeDelta) {
		switch delta.Event {
		case "ready":
			fmt.Printf("✅ Listening on: %s (engine: %s%s)\n", delta.Device, orDefault(delta.Engine, "voxtral"), languageSuffix(delta.Language))
			fmt.Fprintf(logFile, "--- session started, device: %s, engine: %s%s, %s ---\n",
				delta.Device, orDefault(delta.Engine, "voxtral"), languageSuffix(delta.Language), time.Now().Format(time.RFC3339))
		case "delta":
			text := delta.Text
			if delta.Speaker != "" && delta.Speaker != lastSpeaker {
				text = "\n[" + delta.Speaker + "] " + text
				lastSpeaker = delta.Speaker
			}
			h.broadcast(text)
			fmt.Fprint(logFile, text)
		case "done":
			fmt.Fprintf(logFile, "\n--- session ended, %s ---\n", time.Now().Format(time.RFC3339))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to start realtime transcription: %w", err)
	}

	server := &http.Server{Addr: fmt.Sprintf(":%d", opts.port), Handler: newMux(h)}

	procutil.OnInterrupt(func() {
		fmt.Println("\n⏹️  Stopping...")
		stop()
		server.Close()
	})

	fmt.Printf("🌐 Open http://localhost:%d to watch the live transcript\n", opts.port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}
