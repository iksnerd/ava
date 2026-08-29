package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"local-whisper/internal/audio"
	"local-whisper/internal/clipboard"
	"local-whisper/internal/recording"
	"local-whisper/pkg/voxtral"
	"local-whisper/pkg/whisper"
)

const (
	tmpDir       = "/tmp/voice-input"
	baseModel    = "ggml-base.en.bin"
	tinyModel    = "ggml-tiny.en.bin"
)

func main() {
	// Command-line flags
	contextFile := flag.String("context", "", "Path to context file (optional)")
	outputFile := flag.String("output", "", "Output file for transcription (optional)")
	workDir := flag.String("dir", "", "Working directory (optional)")
	modelName := flag.String("model", "base", "Model size: base (default) or tiny (for whisper only)")
	language := flag.String("lang", "en", "Language code: en, es, fr, de, etc. (default: en)")
	noPaste := flag.Bool("no-paste", false, "Don't auto-paste to clipboard/cursor")
	noSound := flag.Bool("no-sound", false, "Disable sound effects")
	showStatus := flag.Bool("verbose", true, "Show processing status")
	engine := flag.String("engine", "whisper", "Inference engine: whisper (default) or voxtral")

	flag.Parse()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if *showStatus {
			fmt.Println("\n⏹️ Recording cancelled.")
		}
		os.Exit(0)
	}()

	// Validate engine selection
	if *engine != "whisper" && *engine != "voxtral" {
		fmt.Fprintf(os.Stderr, "❌ Invalid engine: %s (use 'whisper' or 'voxtral')\n", *engine)
		os.Exit(1)
	}

	// Validate model selection
	if *engine == "whisper" && *modelName != "base" && *modelName != "tiny" {
		fmt.Fprintf(os.Stderr, "❌ Invalid model: %s (use 'base' or 'tiny')\n", *modelName)
		os.Exit(1)
	}

	// Change to working directory if specified
	if *workDir != "" {
		if err := os.Chdir(*workDir); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to change directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Create temp directory
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create temp directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// Check dependencies based on engine
	if err := checkDependencies(*engine); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	if *showStatus {
		fmt.Printf("🎤 Starting voice transcription (engine: %s)...\n", *engine)
	}

	// Step 1: Record audio
	if *showStatus {
		fmt.Println("🎧 Listening... (press Ctrl+C to stop)")
	}

	rawAudioPath := filepath.Join(tmpDir, "prompt_raw.wav")
	recorder := recording.NewRecorder(rawAudioPath, !*noSound)
	if err := recorder.Record(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Recording failed: %v\n", err)
		os.Exit(1)
	}

	// Check if audio was actually recorded
	fileInfo, err := os.Stat(rawAudioPath)
	if err != nil || fileInfo.Size() < 1000 {
		fmt.Println("⚠️ No audio recorded.")
		os.Exit(0)
	}

	if *showStatus {
		fmt.Println("✅ Raw audio recorded.")
	}

	// Step 2: Process audio
	if *showStatus {
		fmt.Println("🎧 Normalizing audio...")
	}

	processedAudioPath := filepath.Join(tmpDir, "prompt_processed.wav")
	processor := audio.NewProcessor(rawAudioPath, processedAudioPath)
	if err := processor.Normalize(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Audio processing failed: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Load context
	contextPrompt := ". "
	if *contextFile != "" {
		if content, err := os.ReadFile(*contextFile); err == nil {
			contextPrompt = string(content) + " " + contextPrompt
			if *showStatus {
				fmt.Printf("📂 Context loaded: %s\n", *contextFile)
			}
		}
	} else {
		// Try to load from home directory
		homeDir, err := os.UserHomeDir()
		if err == nil {
			globalContextPath := filepath.Join(homeDir, ".whisper-context")
			if content, err := os.ReadFile(globalContextPath); err == nil {
				contextPrompt = string(content) + " " + contextPrompt
				if *showStatus {
					fmt.Println("🌍 Global context loaded: ~/.whisper-context")
				}
			}
		}
	}

	// Step 4: Transcribe
	if *showStatus {
		fmt.Println("🧠 Transcribing audio...")
	}

	var text string
	var transcribeErr error
	textOutputPath := filepath.Join(tmpDir, "prompt.txt")

	if *engine == "voxtral" {
		vxClient := voxtral.NewClient("") // Defaults to http://127.0.0.1:8765
		text, transcribeErr = vxClient.Transcribe(voxtral.TranscribeOptions{
			AudioPath:     processedAudioPath,
			OutputPath:    textOutputPath,
			ContextPrompt: contextPrompt,
			Language:      *language,
		})
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to get home directory: %v\n", err)
			os.Exit(1)
		}

		// Select model
		var modelFile string
		if *modelName == "tiny" {
			modelFile = tinyModel
		} else {
			modelFile = baseModel
		}

		modelPath := filepath.Join(homeDir, ".local/share/whisper-cpp", modelFile)
		whisperClient := whisper.NewClient(modelPath)

		text, transcribeErr = whisperClient.Transcribe(whisper.TranscribeOptions{
			AudioPath:     processedAudioPath,
			OutputPath:    textOutputPath,
			ContextPrompt: contextPrompt,
			Language:      *language,
		})
	}

	if transcribeErr != nil {
		fmt.Fprintf(os.Stderr, "❌ Transcription failed: %v\n", transcribeErr)
		os.Exit(1)
	}

	if text == "" {
		fmt.Println("⚠️ No speech detected.")
		os.Exit(0)
	}

	// Step 5: Output
	fmt.Printf("✅ Copied: %s\n", text)

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(text), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to write output file: %v\n", err)
			os.Exit(1)
		}
		if *showStatus {
			fmt.Printf("📝 Saved to: %s\n", *outputFile)
		}
	}

	// Copy to clipboard and paste if not disabled
	if !*noPaste {
		if err := clipboard.CopyToClipboard(text); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ Failed to copy to clipboard: %v\n", err)
		} else {
			// Auto-paste using AppleScript
			clipboard.PasteWithAppleScript()
		}
	}

	// Play completion sound
	clipboard.PlaySound("/System/Library/Sounds/Pop.aiff", !*noSound)
}

func checkDependencies(engine string) error {
	deps := []string{"sox"}
	if engine == "whisper" {
		deps = append(deps, "whisper-cli")
	}

	for _, dep := range deps {
		_, err := exec.LookPath(dep)
		if err != nil {
			if dep == "sox" {
				return fmt.Errorf("sox is not installed. Run: brew install sox")
			} else if dep == "whisper-cli" {
				return fmt.Errorf("whisper-cli is not installed. Run: brew install whisper-cpp")
			}
		}
	}

	if engine == "voxtral" {
		// Quick HTTP GET to see if the server and model are up
		client := &http.Client{Timeout: 1 * time.Second}
		res, err := client.Get("http://127.0.0.1:8765/health")
		if err != nil || res.StatusCode != 200 {
			return fmt.Errorf("voxtral server is not running or model failed to load. Start it by running: bash scripts/voxtral-server.sh start")
		}
		res.Body.Close()
	}

	return nil
}
