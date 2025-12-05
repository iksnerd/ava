package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	whisperModel = "ggml-base.en.bin"
	tmpDir       = "/tmp/voice-input"
)

func main() {
	// Command-line flags
	contextFile := flag.String("context", "", "Path to context file (optional)")
	outputFile := flag.String("output", "", "Output file for transcription (optional)")
	workDir := flag.String("dir", "", "Working directory (optional)")
	modelName := flag.String("model", "base", "Model size: base (default) or tiny")
	language := flag.String("lang", "en", "Language code: en, es, fr, de, etc. (default: en)")
	noPaste := flag.Bool("no-paste", false, "Don't auto-paste to clipboard/cursor")
	noSound := flag.Bool("no-sound", false, "Disable sound effects")
	showStatus := flag.Bool("verbose", true, "Show processing status")

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

	// Validate model selection
	if *modelName != "base" && *modelName != "tiny" {
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

	// Check dependencies
	if err := checkDependencies(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	if *showStatus {
		fmt.Println("🎤 Starting voice transcription...")
	}

	// Step 1: Record audio
	if *showStatus {
		fmt.Println("🎧 Listening... (press Ctrl+C to stop)")
	}

	rawAudioPath := filepath.Join(tmpDir, "prompt_raw.wav")
	if err := recordAudio(rawAudioPath, !*noSound); err != nil {
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
	if err := processAudio(rawAudioPath, processedAudioPath); err != nil {
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

	textOutputPath := filepath.Join(tmpDir, "prompt.txt")
	text, err := transcribeAudio(processedAudioPath, textOutputPath, contextPrompt, *modelName, *language)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Transcription failed: %v\n", err)
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
		if err := copyToClipboard(text); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ Failed to copy to clipboard: %v\n", err)
		} else {
			// Auto-paste using AppleScript
			pasteWithAppleScript()
		}
	}

	// Play completion sound
	playSound("/System/Library/Sounds/Pop.aiff", !*noSound)
}

func checkDependencies() error {
	deps := []string{"sox", "whisper-cli"}
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			if dep == "sox" {
				return fmt.Errorf("sox is not installed. Run: brew install sox")
			} else if dep == "whisper-cli" {
				return fmt.Errorf("whisper-cli is not installed. Run: brew install whisper-cpp")
			}
		}
	}
	return nil
}

func recordAudio(outputPath string, playSound bool) error {
	// Play start sound in background (doesn't block recording)
	if playSound {
		go func() {
			cmd := exec.Command("afplay", "/System/Library/Sounds/Blow.aiff")
			cmd.Run()
		}()
	}

	// Record with sox immediately: 
	// - Start recording immediately (skip initial silence)
	// - Stop after 2.0s of silence at 3% threshold (more lenient)
	cmd := exec.Command("sox", "-d", "-r", "16000", "-c", "1", outputPath,
		"silence", "1", "0.01", "0.1%", "1", "2.0", "3%")

	// Show sox output for debugging silence detection
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func processAudio(inputPath, outputPath string) error {
	// Normalize audio with rate conversion to ensure compatibility
	cmd := exec.Command("sox", inputPath, "-r", "16000", "-c", "1", outputPath, "norm", "-3")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func transcribeAudio(audioPath, outputPath, contextPrompt, modelName, language string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}

	// Select model based on flag
	var modelFile string
	if modelName == "tiny" {
		modelFile = "ggml-tiny.en.bin"
	} else {
		modelFile = whisperModel // base model
	}

	modelPath := filepath.Join(homeDir, ".local/share/whisper-cpp", modelFile)

	// Check if model exists
	if _, err := os.Stat(modelPath); err != nil {
		return "", fmt.Errorf("whisper model not found at %s. Download with: wget -O %s https://huggingface.co/ggerganov/whisper.cpp/resolve/main/%s", modelPath, modelPath, modelFile)
	}

	// Transcribe with language parameter
	cmd := exec.Command("whisper-cli",
		"-m", modelPath,
		"-f", audioPath,
		"-otxt", "-of", strings.TrimSuffix(outputPath, ".txt"),
		"-t", "8", "-nt", "-sns",
		"-l", language,
		"--prompt", contextPrompt,
	)

	// Redirect output to stderr (whisper logs to stderr)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Read transcribed text
	text, err := os.ReadFile(outputPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(text)), nil
}

func playSound(soundPath string, enabled bool) {
	if !enabled {
		return
	}
	cmd := exec.Command("afplay", soundPath)
	cmd.Run() // Ignore errors, just background play
}

func pasteWithAppleScript() {
	// Auto-paste using AppleScript: tell application "System Events" to keystroke "v" using {command down}
	script := `tell application "System Events" to keystroke "v" using {command down}`
	cmd := exec.Command("osascript", "-e", script)
	cmd.Run() // Ignore errors
}

func copyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := fmt.Fprint(stdin, text); err != nil {
		return err
	}

	if err := stdin.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}
