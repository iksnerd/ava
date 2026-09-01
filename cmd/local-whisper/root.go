package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"local-whisper/internal/clipboard"
	"local-whisper/internal/procutil"
	"local-whisper/internal/recording"
	"local-whisper/pkg/mlx"
	"local-whisper/pkg/stt"
	"local-whisper/pkg/stt/whisper"
)

const tmpDir = "/tmp/voice-input"

// options holds the parsed --flag values for the root command's RunE.
type options struct {
	contextFile string
	outputFile  string
	workDir     string
	modelName   string
	language    string
	noPaste     bool
	noSound     bool
	showStatus  bool
	engine      string
}

func newRootCmd() *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use:           "local-whisper",
		Short:         "Record audio, transcribe it, and copy/paste the result",
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
	flags.StringVar(&opts.workDir, "dir", "", "Working directory (optional)")
	flags.StringVar(&opts.modelName, "model", "base", "Model size: base (default) or tiny (for whisper only)")
	flags.StringVar(&opts.language, "lang", "en", "Language code: en, es, fr, de, etc. (default: en)")
	flags.BoolVar(&opts.noPaste, "no-paste", false, "Don't auto-paste to clipboard/cursor")
	flags.BoolVar(&opts.noSound, "no-sound", false, "Disable sound effects")
	flags.BoolVar(&opts.showStatus, "verbose", true, "Show processing status")
	flags.StringVar(&opts.engine, "engine", "whisper", "Inference engine: whisper (default) or voxtral")

	cmd.AddCommand(newEngineCmd())

	return cmd
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

// run is the root command's RunE body: record, transcribe, output.
func run(opts options) error {
	if err := validateEngine(opts.engine); err != nil {
		return err
	}
	if err := validateModel(opts.engine, opts.modelName); err != nil {
		return err
	}

	// Change to working directory if specified
	if opts.workDir != "" {
		if err := os.Chdir(opts.workDir); err != nil {
			return fmt.Errorf("failed to change directory: %w", err)
		}
	}

	// Create temp directory
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup signal handling for graceful shutdown: clean up the temp
	// directory (which may hold recorded audio) before exiting, same as a
	// normal run does via the defer above.
	procutil.OnInterrupt(func() {
		if opts.showStatus {
			fmt.Println("\n⏹️ Recording cancelled.")
		}
		os.RemoveAll(tmpDir)
		os.Exit(0)
	})

	// Check dependencies based on engine
	if err := checkDependencies(opts.engine, voxtralHealthURL); err != nil {
		return err
	}

	if opts.showStatus {
		fmt.Printf("🎤 Starting voice transcription (engine: %s)...\n", opts.engine)
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
		fmt.Println("⚠️ No audio recorded.")
		return nil
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

	var transcriber stt.Client
	if opts.engine == "voxtral" {
		transcriber = mlx.NewClient("") // Defaults to http://127.0.0.1:8765
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		modelPath := filepath.Join(homeDir, ".local/share/whisper-cpp", selectModelFile(opts.modelName))
		transcriber = whisper.NewClient(modelPath)
	}

	text, transcribeErr := transcriber.Transcribe(stt.Options{
		AudioPath:     audioPath,
		OutputPath:    filepath.Join(tmpDir, "prompt.txt"),
		ContextPrompt: contextPrompt,
		Language:      opts.language,
	})
	if transcribeErr != nil {
		return fmt.Errorf("transcription failed: %w", transcribeErr)
	}

	if text == "" {
		fmt.Println("⚠️ No speech detected.")
		return nil
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

	// Copy to clipboard and paste if not disabled
	if !opts.noPaste {
		if err := clipboard.CopyToClipboard(text); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ Failed to copy to clipboard: %v\n", err)
		} else {
			// Auto-paste using AppleScript
			clipboard.PasteWithAppleScript()
		}
	}

	// Play completion sound
	clipboard.PlaySound("/System/Library/Sounds/Pop.aiff", !opts.noSound)

	return nil
}
