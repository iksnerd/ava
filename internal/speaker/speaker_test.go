package speaker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/local-whisper/internal/enginedist"
	"github.com/iksnerd/local-whisper/internal/voiceconfig"
	"github.com/iksnerd/local-whisper/pkg/mlx"
)

// stubEngine stands in for the mlx-engine HTTP client.
type stubEngine struct {
	healthy bool
	audio   []byte
	err     error
	calls   []mlx.SpeakOptions
}

func (e *stubEngine) Healthy() bool { return e.healthy }

func (e *stubEngine) Speak(opts mlx.SpeakOptions) ([]byte, error) {
	e.calls = append(e.calls, opts)
	if e.err != nil {
		return nil, e.err
	}
	return e.audio, nil
}

type recorder struct {
	played       []string
	playedVolume float64
	saidText     string
	saidRate     int
	// markerSeen records whether an activity marker existed at playback
	// time — that file is what AvaMenuBar polls and what
	// ttscontrol.StopSpeaking() looks for.
	markerSeen bool
}

// newTestSpeaker wires a Speaker whose every side effect is captured rather
// than performed: no HTTP, no afplay, no `say`, no shared /tmp state.
func newTestSpeaker(t *testing.T, engine Engine, settings voiceconfig.Settings) (*Speaker, *recorder) {
	t.Helper()
	dir := t.TempDir()
	rec := &recorder{}

	s := &Speaker{
		Engine:      engine,
		Settings:    func() voiceconfig.Settings { return settings },
		ActivityDir: filepath.Join(dir, "active"),
		LockPath:    filepath.Join(dir, "playback.lock"),
	}
	s.Play = func(audioPath string, volume float64, pidFile string) error {
		rec.played = append(rec.played, audioPath)
		rec.playedVolume = volume
		rec.markerSeen = hasMarker(s.ActivityDir)
		return nil
	}
	s.Say = func(text string, rate int, outPath string) error {
		rec.saidText = text
		rec.saidRate = rate
		return os.WriteFile(outPath, []byte("fake aiff"), 0644)
	}
	return s, rec
}

func hasMarker(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".play.pid") && !strings.HasSuffix(name, ".stopped") {
			return true
		}
	}
	return false
}

func settingsWith(mutate func(*voiceconfig.Settings)) voiceconfig.Settings {
	s := voiceconfig.Defaults()
	mutate(&s)
	return s
}

// Mute is the single choke point every speak path goes through, so it has
// to win before anything is synthesized — not just before playback.
func TestMuteSkipsSynthesisAndPlayback(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, rec := newTestSpeaker(t, engine, settingsWith(func(s *voiceconfig.Settings) { s.Muted = true }))

	if err := s.Speak("hello", Options{}); err != nil {
		t.Fatalf("Speak() error = %v, want nil when muted", err)
	}
	if len(engine.calls) != 0 {
		t.Errorf("engine called %d times, want 0 when muted", len(engine.calls))
	}
	if len(rec.played) != 0 || rec.saidText != "" {
		t.Errorf("played %v / said %q, want silence when muted", rec.played, rec.saidText)
	}
}

func TestSpeakUsesEngineWhenHealthy(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("RIFFwav")}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())

	if err := s.Speak("hello there", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if len(engine.calls) != 1 {
		t.Fatalf("engine called %d times, want 1", len(engine.calls))
	}
	if engine.calls[0].Text != "hello there" {
		t.Errorf("text = %q, want %q", engine.calls[0].Text, "hello there")
	}
	if len(rec.played) != 1 {
		t.Fatalf("played %d files, want 1", len(rec.played))
	}
	if rec.saidText != "" {
		t.Errorf("fell back to say (%q) despite a healthy engine", rec.saidText)
	}
}

func TestSpeakDefaultsVoiceSpeedAndVolumeFromSettings(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	settings := settingsWith(func(s *voiceconfig.Settings) {
		s.Voice = "bf_emma"
		s.Speed = 0.9
		s.Volume = 0.4
	})
	s, rec := newTestSpeaker(t, engine, settings)

	if err := s.Speak("hi", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if engine.calls[0].Voice != "bf_emma" || engine.calls[0].Speed != 0.9 {
		t.Errorf("request = %+v, want the configured voice and speed", engine.calls[0])
	}
	if rec.playedVolume != 0.4 {
		t.Errorf("volume = %v, want the configured 0.4", rec.playedVolume)
	}
}

func TestSpeakOptionsOverrideSettings(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())

	if err := s.Speak("hi", Options{Voice: "am_adam", Speed: 1.8}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if engine.calls[0].Voice != "am_adam" || engine.calls[0].Speed != 1.8 {
		t.Errorf("request = %+v, want the per-call overrides", engine.calls[0])
	}
}

func TestSpeakFallsBackToSayWhenEngineUnhealthy(t *testing.T) {
	engine := &stubEngine{healthy: false}
	settings := settingsWith(func(s *voiceconfig.Settings) {
		s.SayRate = 180
		s.EngineAutoStart = false
	})
	s, rec := newTestSpeaker(t, engine, settings)

	if err := s.Speak("no server", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if len(engine.calls) != 0 {
		t.Errorf("engine called despite being unhealthy")
	}
	if rec.saidText != "no server" {
		t.Errorf("said %q, want the fallback to speak the text", rec.saidText)
	}
	if rec.saidRate != 180 {
		t.Errorf("say rate = %d, want the configured 180", rec.saidRate)
	}
	if len(rec.played) != 1 {
		t.Errorf("played %d files, want the rendered `say` output to go through afplay", len(rec.played))
	}
}

func TestSpeakFallsBackToSayWhenSynthesisFails(t *testing.T) {
	engine := &stubEngine{healthy: true, err: errors.New("tts not loaded")}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())

	if err := s.Speak("degraded", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if rec.saidText != "degraded" {
		t.Errorf("said %q, want a fallback after a synthesis failure", rec.saidText)
	}
}

// AvaMenuBar's speaking indicator and ttscontrol.StopSpeaking() both
// key off a marker file that exists for the whole synth+playback window.
func TestSpeakTracksActivityMarkerAndCleansUp(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())

	if err := s.Speak("tracked", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if !rec.markerSeen {
		t.Error("no activity marker existed during playback")
	}

	entries, err := os.ReadDir(s.ActivityDir)
	if err != nil {
		t.Fatalf("read activity dir: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("activity dir still holds %v, want it emptied on exit", names)
	}
}

// A deliberate stop must not degrade into the `say` fallback — the user
// asked for silence, not for a different voice.
func TestStoppedSynthesisDoesNotFallBackToSay(t *testing.T) {
	engine := &stubEngine{healthy: true}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	engine.err = errors.New("cancelled")

	// Simulate ttscontrol.StopSpeaking() landing mid-synthesis: it drops a
	// .stopped sidecar next to whatever marker it finds.
	s.Engine = &stoppingEngine{inner: engine, speaker: s}

	if err := s.Speak("cancel me", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if rec.saidText != "" {
		t.Errorf("said %q, want silence after a deliberate stop", rec.saidText)
	}
	if len(rec.played) != 0 {
		t.Errorf("played %v, want nothing after a deliberate stop", rec.played)
	}
}

// stoppingEngine writes the .stopped sidecar while "synthesizing", the way
// a concurrent StopSpeaking() would.
type stoppingEngine struct {
	inner   Engine
	speaker *Speaker
}

func (e *stoppingEngine) Healthy() bool { return true }

func (e *stoppingEngine) Speak(opts mlx.SpeakOptions) ([]byte, error) {
	entries, _ := os.ReadDir(e.speaker.ActivityDir)
	for _, entry := range entries {
		_ = os.WriteFile(filepath.Join(e.speaker.ActivityDir, entry.Name()+".stopped"), nil, 0644)
	}
	return nil, errors.New("cancelled")
}

// Falling back to `say` changes the voice the user hears and degrades quality.
// Staying quiet about it makes "why does it sound robotic / why is it the wrong
// voice" unanswerable from the outside, which is exactly what happened before.
func TestSpeakWarnsWhenItFallsBackToSay(t *testing.T) {
	var notice strings.Builder
	s, rec := newTestSpeaker(t, &stubEngine{healthy: false}, voiceconfig.Settings{})
	s.Notice = &notice
	s.AutoStart = func() error { return errors.New("mlx-engine not installed") }

	if err := s.Speak("build finished", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}

	if rec.saidText != "build finished" {
		t.Errorf("say got %q, want the original text", rec.saidText)
	}
	got := notice.String()
	for _, want := range []string{"say", "engine start"} {
		if !strings.Contains(got, want) {
			t.Errorf("fallback warning %q should mention %q so the user can act on it", got, want)
		}
	}
}

// The happy path must stay quiet: a warning on every successful sentence would
// train the user to ignore the one that matters.
func TestSpeakIsSilentWhenTheEngineWorks(t *testing.T) {
	var notice strings.Builder
	s, _ := newTestSpeaker(t, &stubEngine{healthy: true, audio: []byte("RIFF")}, voiceconfig.Settings{})
	s.Notice = &notice

	if err := s.Speak("all good", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if notice.Len() != 0 {
		t.Errorf("successful synthesis should print no warning, got %q", notice.String())
	}
}

// A release install has no checkout, so auto-start has to find the bundle
// `local-whisper setup` wrote, or every line falls back to the macOS voice.
func TestEngineScriptFindsTheInstalledBundle(t *testing.T) {
	t.Setenv(EngineScriptEnv, "")
	t.Chdir(t.TempDir()) // no scripts/ here, as for a binary in ~/.local/bin

	dir := t.TempDir()
	t.Setenv(enginedist.DirEnv, dir)
	installed := filepath.Join(dir, enginedist.ScriptRelPath)
	if err := os.MkdirAll(filepath.Dir(installed), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}

	if got := engineScript(); got != installed {
		t.Errorf("engineScript() = %q, want the installed bundle %q", got, installed)
	}
}

func TestEngineScriptPrefersTheOverride(t *testing.T) {
	t.Setenv(EngineScriptEnv, "/custom/mlx-engine-server.sh")
	if got := engineScript(); got != "/custom/mlx-engine-server.sh" {
		t.Errorf("engineScript() = %q, want the %s override", got, EngineScriptEnv)
	}
}
