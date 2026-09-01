package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"local-whisper/internal/clipboard"
	"local-whisper/internal/procutil"
	"local-whisper/internal/recording"
	"local-whisper/pkg/mlx"
	"local-whisper/pkg/stt"
	"local-whisper/pkg/stt/whisper"
)

const (
	tmpDir           = "/tmp/voice-input"
	baseModel        = "ggml-base.en.bin"
	tinyModel        = "ggml-tiny.en.bin"
	voxtralHealthURL = "http://127.0.0.1:8765/health"
	noContextPrompt  = ". "
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

	if err := validateEngine(*engine); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	if err := validateModel(*engine, *modelName); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
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

	// Setup signal handling for graceful shutdown: clean up the temp
	// directory (which may hold recorded audio) before exiting, same as a
	// normal run does via the defer above.
	procutil.OnInterrupt(func() {
		if *showStatus {
			fmt.Println("\n⏹️ Recording cancelled.")
		}
		os.RemoveAll(tmpDir)
		os.Exit(0)
	})

	// Check dependencies based on engine
	if err := checkDependencies(*engine, voxtralHealthURL); err != nil {
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

	audioPath := filepath.Join(tmpDir, "prompt.wav")
	recorder := recording.NewRecorder(audioPath, !*noSound)
	if err := recorder.Record(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Recording failed: %v\n", err)
		os.Exit(1)
	}

	// Check if audio was actually recorded
	fileInfo, err := os.Stat(audioPath)
	if err != nil || fileInfo.Size() < 1000 {
		fmt.Println("⚠️ No audio recorded.")
		os.Exit(0)
	}

	if *showStatus {
		fmt.Println("✅ Audio recorded.")
	}

	// Step 2: Load context
	var globalContextPath string
	if homeDir, err := os.UserHomeDir(); err == nil {
		globalContextPath = filepath.Join(homeDir, ".whisper-context")
	}
	contextPrompt, contextMsg := loadContextPrompt(*contextFile, globalContextPath)
	if *showStatus && contextMsg != "" {
		fmt.Println(contextMsg)
	}

	// Step 3: Transcribe
	if *showStatus {
		fmt.Println("🧠 Transcribing audio...")
	}

	var transcriber stt.Client
	if *engine == "voxtral" {
		transcriber = mlx.NewClient("") // Defaults to http://127.0.0.1:8765
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to get home directory: %v\n", err)
			os.Exit(1)
		}
		modelPath := filepath.Join(homeDir, ".local/share/whisper-cpp", selectModelFile(*modelName))
		transcriber = whisper.NewClient(modelPath)
	}

	text, transcribeErr := transcriber.Transcribe(stt.Options{
		AudioPath:     audioPath,
		OutputPath:    filepath.Join(tmpDir, "prompt.txt"),
		ContextPrompt: contextPrompt,
		Language:      *language,
	})
	if transcribeErr != nil {
		fmt.Fprintf(os.Stderr, "❌ Transcription failed: %v\n", transcribeErr)
		os.Exit(1)
	}

	if text == "" {
		fmt.Println("⚠️ No speech detected.")
		os.Exit(0)
	}

	// Step 4: Output
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

// validateEngine checks that engine is one of the supported inference
// engines.
func validateEngine(engine string) error {
	if engine != "whisper" && engine != "voxtral" {
		return fmt.Errorf("Invalid engine: %s (use 'whisper' or 'voxtral')", engine)
	}
	return nil
}

// validateModel checks the --model flag; it only constrains the whisper
// engine, which ships exactly two local models.
func validateModel(engine, model string) error {
	if engine == "whisper" && model != "base" && model != "tiny" {
		return fmt.Errorf("Invalid model: %s (use 'base' or 'tiny')", model)
	}
	return nil
}

// selectModelFile maps the --model flag to the whisper.cpp model filename
// under ~/.local/share/whisper-cpp/.
func selectModelFile(modelName string) string {
	if modelName == "tiny" {
		return tinyModel
	}
	return baseModel
}

// loadContextPrompt resolves the transcription context prompt: an explicit
// --context file takes priority, then the per-user global context file
// (globalContextPath, normally ~/.whisper-context), falling back to no
// context. status is a human-readable line for --verbose output, empty if
// nothing was loaded.
func loadContextPrompt(contextFile, globalContextPath string) (prompt, status string) {
	if contextFile != "" {
		if content, err := os.ReadFile(contextFile); err == nil {
			return string(content) + " " + noContextPrompt, fmt.Sprintf("📂 Context loaded: %s", contextFile)
		}
		return noContextPrompt, ""
	}
	if globalContextPath != "" {
		if content, err := os.ReadFile(globalContextPath); err == nil {
			return string(content) + " " + noContextPrompt, "🌍 Global context loaded: ~/.whisper-context"
		}
	}
	return noContextPrompt, ""
}

func checkDependencies(engine, voxtralHealthURL string) error {
	deps := []string{"sox"}
	if engine == "whisper" {
		deps = append(deps, "whisper-cli")
	}

	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			switch dep {
			case "sox":
				return fmt.Errorf("sox is not installed. Run: brew install sox")
			case "whisper-cli":
				return fmt.Errorf("whisper-cli is not installed. Run: brew install whisper-cpp")
			default:
				return fmt.Errorf("%s is not installed", dep)
			}
		}
	}

	if engine == "voxtral" {
		// Quick HTTP GET to see if the server and model are up
		client := &http.Client{Timeout: 1 * time.Second}
		res, err := client.Get(voxtralHealthURL)
		if err != nil {
			return fmt.Errorf("voxtral server is not running or model failed to load. Start it by running: bash scripts/mlx-engine-server.sh start")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("voxtral server is not running or model failed to load. Start it by running: bash scripts/mlx-engine-server.sh start")
		}
	}

	return nil
}
