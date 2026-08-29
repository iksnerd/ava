// Command voice-monitor runs Voxtral Mini 4B Realtime against an input
// device (the mic, or a loopback device like BlackHole capturing a call)
// and serves the live transcript on localhost while logging it to a file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"local-whisper/pkg/voxtral"
)

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func languageSuffix(language string) string {
	if language == "" || language == "en" {
		return ""
	}
	return ", language: " + language
}

// hub fans transcript deltas out to any connected /events (SSE) clients and
// keeps the full transcript so far so a newly opened tab sees history.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
	history []byte
}

func newHub() *hub {
	return &hub{clients: make(map[chan []byte]bool)}
}

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *hub) broadcast(text string) {
	payload, _ := json.Marshal(map[string]string{"text": text})

	h.mu.Lock()
	h.history = append(h.history, text...)
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
			// Slow client - drop this update rather than block the pipeline.
		}
	}
	h.mu.Unlock()
}

func (h *hub) snapshot() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.history)
}

const indexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Voxtral Realtime Monitor</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #111; color: #eee; }
  header { padding: 12px 20px; background: #1b1b1b; border-bottom: 1px solid #333; }
  header .status { font-size: 13px; color: #8f8; }
  header .status.disconnected { color: #f88; }
  main { padding: 20px; }
  #transcript { white-space: pre-wrap; font-size: 18px; line-height: 1.6; max-width: 800px; }
  .hint { color: #888; font-size: 13px; margin-top: 20px; }
</style>
</head>
<body>
<header>
  <strong>Voxtral Realtime Monitor</strong>
  &mdash; <span id="status" class="status disconnected">connecting…</span>
</header>
<main>
  <div id="transcript"></div>
  <div class="hint">Live transcript from Voxtral Mini 4B Realtime. Also being written to the transcript log file on disk.</div>
</main>
<script>
  const el = document.getElementById('transcript');
  const status = document.getElementById('status');
  const es = new EventSource('/events');
  es.onopen = () => { status.textContent = 'connected'; status.className = 'status'; };
  es.onerror = () => { status.textContent = 'disconnected'; status.className = 'status disconnected'; };
  es.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    el.textContent += msg.text;
    window.scrollTo(0, document.body.scrollHeight);
  };
</script>
</body>
</html>`

func main() {
	device := flag.String("device", "", "Input device name substring (e.g. BlackHole) or index. Default: system mic.")
	engine := flag.String("engine", "", "STT engine: 'voxtral' (default, <500ms, 13 languages) or 'whisper' (multilingual incl. Bulgarian, ~1s latency)")
	sttModel := flag.String("stt-model", "", "Override the engine's default HF repo id")
	language := flag.String("language", "", "Language code, e.g. 'bg' for Bulgarian (--engine whisper only)")
	diarize := flag.Bool("diarize", false, "Tag transcript with speaker labels (--engine whisper only; remote participants only, never your own mic)")
	highpassHz := flag.Float64("highpass-hz", 0, "High-pass filter cutoff in Hz before STT/diarization (0 = use realtime.py's default of 80Hz)")
	port := flag.Int("port", 8765, "Local HTTP port to serve the live transcript on")
	logPath := flag.String("log", "", "Path to write the transcript log (default: /tmp/voice-input/transcript-<timestamp>.txt)")
	pythonPath := flag.String("python", "", "Path to the Voxtral venv python3 (default: <voxtral-dir>/.venv/bin/python3)")
	voxtralDir := flag.String("voxtral-dir", "voxtral", "Path to the voxtral/ primitives directory")
	listDevices := flag.Bool("list-devices", false, "List available input devices and exit")
	flag.Parse()

	resolvedPython := *pythonPath
	if resolvedPython == "" {
		resolvedPython = filepath.Join(*voxtralDir, ".venv", "bin", "python3")
	}

	client := voxtral.NewClient(resolvedPython, *voxtralDir)

	if *listDevices {
		out, err := client.ListInputDevices()
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)
		return
	}

	resolvedLog := *logPath
	if resolvedLog == "" {
		if err := os.MkdirAll("/tmp/voice-input", 0755); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to create /tmp/voice-input: %v\n", err)
			os.Exit(1)
		}
		resolvedLog = filepath.Join("/tmp/voice-input", fmt.Sprintf("transcript-%s.txt", time.Now().Format("20060102-150405")))
	}

	logFile, err := os.OpenFile(resolvedLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	h := newHub()

	fmt.Println("🎧 Starting Voxtral Realtime monitor...")
	fmt.Printf("📝 Logging transcript to: %s\n", resolvedLog)

	realtimeOpts := voxtral.RealtimeOptions{
		Device:     *device,
		Engine:     *engine,
		Model:      *sttModel,
		Language:   *language,
		Diarize:    *diarize,
		HighpassHz: *highpassHz,
	}

	var lastSpeaker string

	stop, err := client.StreamRealtime(realtimeOpts, func(delta voxtral.RealtimeDelta) {
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
		fmt.Fprintf(os.Stderr, "❌ Failed to start realtime transcription: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush() // send headers now so the browser's EventSource fires onopen immediately, not on the first delta

		if snap := h.snapshot(); snap != "" {
			payload, _ := json.Marshal(map[string]string{"text": snap})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		ch := h.subscribe()
		defer h.unsubscribe(ch)

		for {
			select {
			case payload, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: mux}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n⏹️  Stopping...")
		stop()
		server.Close()
	}()

	fmt.Printf("🌐 Open http://localhost:%d to watch the live transcript\n", *port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
		os.Exit(1)
	}
}
