package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/audio"
	"github.com/iksnerd/ava/internal/buildinfo"
	"github.com/iksnerd/ava/internal/clipboard"
	"github.com/iksnerd/ava/internal/procutil"
	"github.com/iksnerd/ava/internal/recording"
	"github.com/iksnerd/ava/pkg/stt"
	"github.com/iksnerd/ava/pkg/stt/whisper"
)

// newDictationDir makes this run's own directory under root and returns a
// cleanup that removes only that. root is internal/audio's TempDir, shared
// with ava-monitor, which keeps its call transcripts there: removing the
// whole root, as dictation used to, unlinked a live call's transcript. A
// directory per run also keeps two dictations from sharing one prompt.wav.
func newDictationDir(root string) (dir string, cleanup func(), err error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create %s: %w", root, err)
	}
	dir, err = os.MkdirTemp(root, "dictate-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// options holds the parsed --flag values for the root command's RunE.
type options struct {
	contextFile string
	outputFile  string
	workDir     string
	modelName   string
	language    string
	beamSize    int
	noPaste     bool
	noSound     bool
	showStatus  bool
}

func newRootCmd() *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use:   "ava",
		Short: "Record audio, transcribe it, and copy/paste the result",
		Long: "Ava: on-device dictation and speech for macOS.\n\n" +
			"With no command, ava dictates: it records until you stop talking, transcribes\n" +
			"with whisper.cpp, and pastes the text at your cursor.\n\n" +
			"First time on this Mac:\n" +
			"  ava setup            install sox, whisper-cli, the speech model and the Kokoro voice\n" +
			"  ava speak \"hello\"    check that speech works\n" +
			"  ava                  dictate; macOS asks once for Microphone (for the app you\n" +
			"                       run it from) and Accessibility (to paste)\n\n" +
			"Guide: https://github.com/iksnerd/ava/blob/main/docs/getting-started.md",
		Version:       buildinfo.Get(),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.contextFile, "context", "", "Path to context file (optional)")
	flags.StringVar(&opts.outputFile, "output", "", "Output file for transcription (optional)")
	flags.StringVar(&opts.workDir, "dir", "", "Run as if started from this directory (affects --context discovery)")
	flags.StringVar(&opts.modelName, "model", "base", "Model size: base or tiny")
	flags.StringVar(&opts.language, "lang", "en", "Language code: en, es, fr, de, etc.")
	flags.IntVar(&opts.beamSize, "beam-size", 0, beamSizeUsage)
	flags.BoolVar(&opts.noPaste, "no-paste", false, "Copy to the clipboard but don't paste")
	flags.BoolVar(&opts.noSound, "no-sound", false, "Disable sound effects")
	flags.BoolVar(&opts.showStatus, "verbose", true, "Show processing status")
	// --verbose already defaults to true, so the only way to get a quiet run was
	// `--verbose=false`. Every other command in this binary spells that --quiet
	// (a11y), so offer it here too rather than having one idea with two opposite
	// spellings. --verbose stays for anything already passing it.
	var quiet bool
	flags.BoolVar(&quiet, "quiet", false, "Suppress processing status (inverse of --verbose)")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		if quiet {
			opts.showStatus = false
		}
	}

	cmd.AddCommand(
		newEngineCmd(),
		newSetupCmd(),
		newSetupModelCmd(),
		newMcpCmd(),
		newSpeakCmd(),
		newStopCmd(),
		newVoicesCmd(),
		newTranscribeCmd(),
		newA11yCmd(),
	)

	return cmd
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

// newTranscriber builds the STT client. Shared by the root command's dictation
// run and the MCP server's transcribe tool, so the two can never disagree about
// where the whisper model lives.
//
// There used to be a second engine here: mlx-engine's Voxtral /transcribe,
// behind --engine voxtral. It was removed after measuring it — on the same
// 20s sample whisper.cpp took 1.26s and Voxtral 17s warm, 127s cold including
// a 108s model load, for a near-identical transcript. mlx-engine is TTS-only
// now; Voxtral still runs in ava-monitor, which needs streaming rather than
// one subprocess per complete file.
func newTranscriber(modelName string) (stt.Client, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	modelPath := filepath.Join(homeDir, ".local/share/whisper-cpp", selectModelFile(modelName))
	return whisper.NewClient(modelPath), nil
}

// run is the root command's RunE body: record, transcribe, output.
func run(opts options) error {
	if err := validateModel(opts.modelName); err != nil {
		return err
	}
	if err := validateBeamSize(opts.beamSize); err != nil {
		return err
	}

	// Change to working directory if specified
	if opts.workDir != "" {
		if err := os.Chdir(opts.workDir); err != nil {
			return fmt.Errorf("failed to change directory: %w", err)
		}
	}

	tmpDir, cleanup, err := newDictationDir(audio.TempDir)
	if err != nil {
		return err
	}
	defer cleanup()

	// Setup signal handling for graceful shutdown: clean up the temp
	// directory (which may hold recorded audio) before exiting, same as a
	// normal run does via the defer above.
	procutil.OnInterrupt(func() {
		if opts.showStatus {
			fmt.Println("\n⏹️ Recording cancelled.")
		}
		cleanup()
		os.Exit(0)
	})

	// Check dependencies based on engine
	if err := checkDependencies(); err != nil {
		return err
	}

	if opts.showStatus {
		fmt.Println("🎤 Starting voice transcription...")
	}

	// Step 1: Record audio
	if opts.showStatus {
		fmt.Println("🎧 Listening... (press Ctrl+C to stop)")
	}

	audioPath := filepath.Join(tmpDir, "prompt.wav")
	recorder := recording.NewRecorder(audioPath, !opts.noSound)
	if err := recorder.Record(); err != nil {
		return fmt.Errorf("recording failed: %w", err)
	}

	// Check if audio was actually recorded
	fileInfo, err := os.Stat(audioPath)
	if err != nil || fileInfo.Size() < 1000 {
		// Returning nil here meant a dead microphone exited 0, so no hotkey
		// wrapper or script could tell it apart from a successful run. The
		// overwhelmingly common cause is the Microphone permission, which is
		// silent: sox "succeeds" and writes nothing.
		return fmt.Errorf("no audio recorded — check Microphone permission for " +
			"whatever you ran this from (System Settings → Privacy & Security → " +
			"Microphone), or pick an input device in Sound settings")
	}

	if opts.showStatus {
		fmt.Println("✅ Audio recorded.")
	}

	// Step 2: Load context
	var globalContextPath string
	if homeDir, err := os.UserHomeDir(); err == nil {
		globalContextPath = filepath.Join(homeDir, ".whisper-context")
	}
	contextPrompt, contextMsg := loadContextPrompt(opts.contextFile, globalContextPath)
	if opts.showStatus && contextMsg != "" {
		fmt.Println(contextMsg)
	}

	// Step 3: Transcribe
	if opts.showStatus {
		fmt.Println("🧠 Transcribing audio...")
	}

	transcriber, err := newTranscriber(opts.modelName)
	if err != nil {
		return err
	}

	text, transcribeErr := transcriber.Transcribe(stt.Options{
		AudioPath:     audioPath,
		OutputPath:    filepath.Join(tmpDir, "prompt.txt"),
		ContextPrompt: contextPrompt,
		Language:      opts.language,
		BeamSize:      opts.beamSize,
	})
	if transcribeErr != nil {
		return fmt.Errorf("transcription failed: %w", transcribeErr)
	}

	if text == "" {
		// Same reasoning as "no audio recorded": exiting 0 made a silent
		// failure undetectable from outside.
		return fmt.Errorf("no speech detected in the recording — it captured " +
			"audio but no words; try speaking closer to the mic, or check the " +
			"input level in Sound settings")
	}

	// Step 4: Output
	fmt.Printf("✅ Copied: %s\n", text)

	if opts.outputFile != "" {
		if err := os.WriteFile(opts.outputFile, []byte(text), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		if opts.showStatus {
			fmt.Printf("📝 Saved to: %s\n", opts.outputFile)
		}
	}

	if err := systemClipboard.deliver(text, !opts.noPaste); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Failed to copy to clipboard: %v\n", err)
	}

	// Play completion sound
	clipboard.PlaySound("/System/Library/Sounds/Pop.aiff", !opts.noSound)

	return nil
}

// clipboardOps is the copy and paste the dictation ends with, as functions so
// a test can check which of them ran.
type clipboardOps struct {
	copy  func(text string) error
	paste func()
}

var systemClipboard = clipboardOps{
	copy:  clipboard.CopyToClipboard,
	paste: clipboard.PasteWithAppleScript,
}

// deliver always copies; paste only adds the keystroke into the focused app,
// so --no-paste still leaves the transcript on the clipboard.
func (c clipboardOps) deliver(text string, paste bool) error {
	if err := c.copy(text); err != nil {
		return err
	}
	if paste {
		c.paste()
	}
	return nil
}
