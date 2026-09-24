// Package monitor is `ava monitor`: it runs Voxtral Mini 4B Realtime against
// an input device (the mic, or a loopback device like BlackHole capturing a
// call) and serves the live transcript on localhost while logging it to a
// file.
package monitor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/audio"
	"github.com/iksnerd/ava/pkg/stt/realtime"
)

// clientOptions holds the persistent --python/--voxtral-dir flags, shared by
// monitor and its devices subcommand since both need a realtime.Client to
// talk to voxtral/realtime.py.
type clientOptions struct {
	pythonPath string
	voxtralDir string
}

func (o clientOptions) client() *realtime.Client {
	return realtime.NewClient(resolvePythonPath(o.pythonPath, o.voxtralDir), o.voxtralDir)
}

// watchOptions holds monitor's own --flag values (the live-watch session), on
// top of the shared clientOptions.
type watchOptions struct {
	clientOptions
	device     string
	engine     string
	sttModel   string
	language   string
	diarize    bool
	highpassHz float64
	port       int
	logPath    string
}

// NewCmd builds `ava monitor` and its devices subcommand.
func NewCmd() *cobra.Command {
	var opts watchOptions

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Serve a live realtime transcript (mic or loopback device) over SSE, logged to a file",
		Long: "Serve a live realtime transcript (mic or loopback device) over SSE at\n" +
			"http://127.0.0.1:8766, logged to a file.\n\n" +
			"Needs the Voxtral Python environment from a checkout of the repo: run\n" +
			"`make setup-voxtral` there once, then run `ava monitor` from the checkout's\n" +
			"root, or pass --voxtral-dir <checkout>/voxtral from anywhere else.\n\n" +
			"Guide: https://github.com/iksnerd/ava/blob/main/docs/monitor.md",
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
	// Not 8765: that is mlx-engine's, and it is running whenever anything has
	// used --engine voxtral or spoken (see CLAUDE.md's Ports convention).
	flags.IntVar(&opts.port, "port", 8766, "Local HTTP port to serve the live transcript on")
	flags.StringVar(&opts.logPath, "log", "", fmt.Sprintf("Path to write the transcript log (default: %s/transcript-<timestamp>.txt)", audio.TempDir))

	cmd.AddCommand(newDevicesCmd(&opts.clientOptions))

	return cmd
}

// watch is monitor's RunE body: stream realtime deltas to the log
// file and to any browser tabs connected over SSE.
// interruptGrace is how long watch waits, after the transcriber exits, for a
// Ctrl-C that was sent at the same moment.
const interruptGrace = 500 * time.Millisecond

func watch(opts watchOptions) error {
	// Registered before realtime.py starts, so no Ctrl-C can arrive unwatched.
	interrupt, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	client := opts.client()

	resolvedLog := opts.logPath
	if resolvedLog == "" {
		if err := os.MkdirAll(audio.TempDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", audio.TempDir, err)
		}
		resolvedLog = defaultLogPath(audio.TempDir, time.Now())
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

	stream, err := client.StreamRealtime(realtimeOpts, func(delta realtime.RealtimeDelta) {
		switch delta.Event {
		case "ready":
			fmt.Printf("✅ Listening on: %s (engine: %s%s)\n", delta.Device, orDefault(delta.Engine, "voxtral"), languageSuffix(delta.Language))
			// The page used to show a generic line that did not even name the
			// log file it told you about, so a silent capture device looked
			// identical to a working one.
			h.setSession(fmt.Sprintf("Listening on %s · engine %s%s · logging to %s",
				delta.Device, orDefault(delta.Engine, "voxtral"), languageSuffix(delta.Language), resolvedLog))
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

	server := &http.Server{Addr: listenAddr(opts.port), Handler: newMux(h)}

	// Ctrl-C stops the transcriber and the server. A transcriber that dies on
	// its own takes the server down too: serving on left the page reading
	// "connected" over a transcript that had stopped, and a closed server
	// makes it say "disconnected".
	go func() {
		select {
		case <-interrupt.Done():
			fmt.Println("\n⏹️  Stopping...")
			stream.Stop()
		case <-stream.Done():
		}
		server.Close()
	}()

	fmt.Printf("🌐 Open http://localhost:%d to watch the live transcript\n", opts.port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		stream.Stop()
		return fmt.Errorf("server error: %w", err)
	}
	if interrupt.Err() != nil {
		return nil
	}
	<-stream.Done()
	// A terminal's Ctrl-C reaches realtime.py and this process at once, and
	// realtime.py can exit first. Give the signal a moment to land before
	// calling the exit a crash.
	select {
	case <-interrupt.Done():
		return nil
	case <-time.After(interruptGrace):
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("transcription stopped: realtime.py exited: %w", err)
	}
	return fmt.Errorf("transcription stopped: realtime.py exited on its own")
}

// listenAddr binds the transcript server to loopback only. It used to be
// ":port", which listens on every interface: anyone on the same network could
// read a live call transcript, with no authentication, while the docs and
// SECURITY.md said it stayed on this machine.
func listenAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}
