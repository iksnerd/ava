package speaker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/internal/voiceconfig"
	"github.com/iksnerd/ava/pkg/mlx"
	"github.com/iksnerd/ava/pkg/stt/captions"
)

// stubEngine stands in for the mlx-engine HTTP client.
type stubEngine struct {
	// onCheck, when set, runs at the start of every Check, so a test can end a
	// context at the moment the speaker is asking after the engine.
	onCheck func()
	healthy bool
	// stale is an engine that answers /health but is not on our protocol.
	stale bool
	audio []byte
	err   error
	calls []mlx.SpeakOptions
	// timed is what SpeakTimed returns; timedCalls records what it was asked.
	timed      mlx.TimedSpeech
	timedCalls []mlx.SpeakOptions
	// timedFor, when set, answers SpeakTimed per text instead of timed, and
	// may fail for one line of a batch.
	timedFor func(text string) (mlx.TimedSpeech, error)
}

func (e *stubEngine) Healthy() bool { return e.healthy }

func (e *stubEngine) SpeakTimed(ctx context.Context, opts mlx.SpeakOptions) (mlx.TimedSpeech, error) {
	e.timedCalls = append(e.timedCalls, opts)
	if e.timedFor != nil {
		return e.timedFor(opts.Text)
	}
	return e.timed, e.err
}

func (e *stubEngine) Check(ctx context.Context) mlx.State {
	if e.onCheck != nil {
		e.onCheck()
	}
	switch {
	case e.stale:
		return mlx.Stale
	case e.healthy:
		return mlx.Ready
	}
	return mlx.Down
}

func (e *stubEngine) Speak(ctx context.Context, opts mlx.SpeakOptions) ([]byte, error) {
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
	s.Play = func(ctx context.Context, audioPath string, volume float64, pidFile string) error {
		rec.played = append(rec.played, audioPath)
		rec.playedVolume = volume
		rec.markerSeen = hasMarker(s.ActivityDir)
		return nil
	}
	s.Say = func(ctx context.Context, text string, rate int, outPath string) error {
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

	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "hello there", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "hi", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "hi", Options{Voice: "am_adam", Speed: 1.8}); err != nil {
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

	if err := s.Speak(context.Background(), "no server", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "degraded", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "tracked", Options{}); err != nil {
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

	if err := s.Speak(context.Background(), "cancel me", Options{}); err != nil {
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

func (e *stoppingEngine) Check(ctx context.Context) mlx.State { return mlx.Ready }

func (e *stoppingEngine) SpeakTimed(context.Context, mlx.SpeakOptions) (mlx.TimedSpeech, error) {
	return mlx.TimedSpeech{}, nil
}

func (e *stoppingEngine) Speak(ctx context.Context, opts mlx.SpeakOptions) ([]byte, error) {
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
	s, rec := newTestSpeaker(t, &stubEngine{healthy: false}, settingsWith(func(c *voiceconfig.Settings) { c.EngineAutoStart = true }))
	s.Notice = &notice
	started := 0
	s.AutoStart = func(context.Context) error { started++; return errors.New("mlx-engine not installed") }

	if err := s.Speak(context.Background(), "build finished", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	// With auto-start off (the zero Settings this test used to pass), the
	// fallback happened without AutoStart ever being tried.
	if started != 1 {
		t.Errorf("AutoStart called %d times, want 1 before falling back", started)
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

	if err := s.Speak(context.Background(), "all good", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if notice.Len() != 0 {
		t.Errorf("successful synthesis should print no warning, got %q", notice.String())
	}
}

// A release install has no checkout, so auto-start has to find the bundle
// `ava setup` wrote, or every line falls back to the macOS voice.
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

func autoStartOn(c *voiceconfig.Settings) { c.EngineAutoStart = true }

func TestSpeakAutoStartsTheEngineAndUsesKokoro(t *testing.T) {
	// Down until AutoStart brings it up, like the real server.
	engine := &stubEngine{healthy: false, audio: []byte("RIFF")}
	s, rec := newTestSpeaker(t, engine, settingsWith(autoStartOn))
	started := 0
	s.AutoStart = func(context.Context) error { started++; engine.healthy = true; return nil }

	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if started != 1 {
		t.Errorf("AutoStart called %d times, want 1", started)
	}
	if len(engine.calls) != 1 || rec.saidText != "" {
		t.Errorf("engine calls %d, said %q; want Kokoro once and no `say`", len(engine.calls), rec.saidText)
	}
}

// Auto-start can exit 0 without a server answering (another start already
// in flight, a server that dies right after). That is a fallback, and it has
// to say why.
func TestSpeakFallsBackWhenAutoStartDoesNotBringTheEngineUp(t *testing.T) {
	var notice strings.Builder
	engine := &stubEngine{healthy: false}
	s, rec := newTestSpeaker(t, engine, settingsWith(autoStartOn))
	s.Notice = &notice
	s.AutoStart = func(context.Context) error { return nil }

	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if rec.saidText != "hello" {
		t.Errorf("said %q, want the `say` fallback", rec.saidText)
	}
	if !strings.Contains(notice.String(), "did not come up") {
		t.Errorf("notice %q does not say the engine did not come up", notice.String())
	}
}

// A deliberate `ava engine stop` turns auto-start off; speech must then not
// quietly start the engine again.
func TestSpeakDoesNotAutoStartWhenTheUserStoppedTheEngine(t *testing.T) {
	s, _ := newTestSpeaker(t, &stubEngine{healthy: false}, settingsWith(func(c *voiceconfig.Settings) { c.EngineAutoStart = false }))
	s.AutoStart = func(context.Context) error { t.Error("auto-started although the user stopped the engine"); return nil }

	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatalf("Speak: %v", err)
	}
}

// Auto-start used to discard the control script's output, so a failed start
// reached the user as "exit status 1" with no reason. The script says what
// went wrong (no engine, a crash on startup); that has to come through.
func TestAutoStartFailureCarriesTheScriptsReason(t *testing.T) {
	script := filepath.Join(t.TempDir(), "mlx-engine-server.sh")
	body := "#!/bin/bash\necho '🚀 Starting...'\necho '❌ No mlx-engine to start: run ava setup'\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EngineScriptEnv, script)

	err := startEngine(context.Background())
	if err == nil || !strings.Contains(err.Error(), "No mlx-engine to start") {
		t.Errorf("startEngine(context.Background()) = %v, want the script's own reason", err)
	}
}

// --out renders instead of speaking: the engine's WAV lands in the file and
// nothing plays. The global mute silences playback, not a file someone asked
// for, so it must not stop a script that is narrating to disk.
func TestOutWritesTheEnginesWavAndPlaysNothing(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("RIFFwav")}
	s, rec := newTestSpeaker(t, engine, settingsWith(func(s *voiceconfig.Settings) { s.Muted = true }))
	out := filepath.Join(t.TempDir(), "line.wav")

	if err := s.Speak(context.Background(), "hello there", Options{Out: out}); err != nil {
		t.Fatalf("Speak(Out) error = %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil || string(got) != "RIFFwav" {
		t.Fatalf("file = %q, %v; want the engine's bytes", got, err)
	}
	if len(rec.played) != 0 || rec.saidText != "" {
		t.Errorf("played %v / said %q; --out must not make a sound", rec.played, rec.saidText)
	}
	if hasMarker(s.ActivityDir) {
		t.Error("an activity marker was left behind; nothing was spoken")
	}
}

// With the engine down there is no file and no `say`: the macOS voice is not
// the configured one, and a script muxing the output would not know.
func TestOutFailsWithoutFallbackWhenTheEngineIsDown(t *testing.T) {
	for name, engine := range map[string]*stubEngine{
		"not running":     {healthy: false},
		"synthesis fails": {healthy: true, err: errors.New("boom")},
	} {
		t.Run(name, func(t *testing.T) {
			s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
			out := filepath.Join(t.TempDir(), "line.wav")

			if err := s.Speak(context.Background(), "hello", Options{Out: out}); err == nil {
				t.Fatal("Speak(Out) succeeded with the engine down")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("a file exists after a failed render: %v", err)
			}
			if rec.saidText != "" || len(rec.played) != 0 {
				t.Errorf("fell back to say/playback: said %q played %v", rec.saidText, rec.played)
			}
		})
	}
}

// --out honours --voice and --speed like any other speak.
func TestOutPassesVoiceAndSpeedToTheEngine(t *testing.T) {
	engine := &stubEngine{healthy: true, audio: []byte("wav")}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())

	if err := s.Speak(context.Background(), "hi", Options{Out: filepath.Join(t.TempDir(), "a.wav"), Voice: "bf_emma", Speed: 0.9}); err != nil {
		t.Fatal(err)
	}
	if got := engine.calls[0]; got.Voice != "bf_emma" || got.Speed != 0.9 {
		t.Errorf("engine got voice %q speed %v, want bf_emma 0.9", got.Voice, got.Speed)
	}
}

// An engine left running across an upgrade answers /health, so "not running"
// and an auto-start would both be wrong: the start would fail to bind. Say
// what is wrong and how to fix it, and still speak, through `say`. It must not
// restart the engine itself, which another session may be mid-synthesis on.
func TestStaleEngineFallsBackToSayWithARestartNotice(t *testing.T) {
	engine := &stubEngine{stale: true, audio: []byte("wav")}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	var notice strings.Builder
	s.Notice = &notice
	started := false
	s.AutoStart = func(context.Context) error { started = true; return nil }

	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if len(engine.calls) != 0 {
		t.Errorf("sent /speak to a stale engine %d times", len(engine.calls))
	}
	if started {
		t.Error("auto-started over a running (stale) engine; it cannot bind and may kill another session's speech")
	}
	if rec.saidText != "hello" {
		t.Errorf("said %q, want the say fallback", rec.saidText)
	}
	for _, want := range []string{"older", "ava engine stop", "ava engine start"} {
		if !strings.Contains(notice.String(), want) {
			t.Errorf("notice %q is missing %q", notice.String(), want)
		}
	}
}

// --out has no say fallback, so a stale engine is an error that says what to do.
func TestOutNamesAStaleEngine(t *testing.T) {
	engine := &stubEngine{stale: true}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())

	err := s.Speak(context.Background(), "hello", Options{Out: filepath.Join(t.TempDir(), "a.wav")})
	if err == nil || !strings.Contains(err.Error(), "ava engine stop") {
		t.Fatalf("err = %v, want one that says to restart the engine", err)
	}
}

// --timings writes where each chunk of the text sits in the audio, beside the
// WAV, from the same synthesis. It uses the engine's timed endpoint, so the
// times are exact and the audio is the one they describe.
func TestTimingsWritesTheWavAndTheSegmentsFromOneSynthesis(t *testing.T) {
	engine := &stubEngine{healthy: true, timed: mlx.TimedSpeech{
		Audio: []byte("RIFFtimed"),
		Segments: []captions.Segment{
			{Text: "Once upon a time.", StartMS: 0, EndMS: 1500},
			{Text: "The end.", StartMS: 1500, EndMS: 2000},
		},
	}}
	s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
	dir := t.TempDir()
	out, timings := filepath.Join(dir, "a.wav"), filepath.Join(dir, "a.json")

	if err := s.Speak(context.Background(), "x", Options{Out: out, Timings: timings, SplitPattern: "SPLIT"}); err != nil {
		t.Fatalf("Speak() error = %v", err)
	}
	if wav, _ := os.ReadFile(out); string(wav) != "RIFFtimed" {
		t.Errorf("wav = %q, want the timed synthesis's audio", wav)
	}
	want := `[{"start_ms":0,"end_ms":1500,"text":"Once upon a time."},{"start_ms":1500,"end_ms":2000,"text":"The end."}]`
	if got, _ := os.ReadFile(timings); strings.TrimSpace(string(got)) != want {
		t.Errorf("timings = %s, want %s", got, want)
	}
	if len(engine.calls) != 0 || len(engine.timedCalls) != 1 || engine.timedCalls[0].SplitPattern != "SPLIT" {
		t.Errorf("engine calls = %v / timed = %v, want one timed call carrying the split pattern", engine.calls, engine.timedCalls)
	}
	if len(rec.played) != 0 || rec.saidText != "" {
		t.Errorf("played %v / said %q; --timings must not make a sound", rec.played, rec.saidText)
	}
}

func TestTimingsNeedsOut(t *testing.T) {
	engine := &stubEngine{healthy: true}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())

	err := s.Speak(context.Background(), "x", Options{Timings: filepath.Join(t.TempDir(), "a.json")})
	if err == nil || !strings.Contains(err.Error(), "out") {
		t.Errorf("err = %v, want one saying timings go with out", err)
	}
	if len(engine.timedCalls) != 0 {
		t.Error("synthesized before refusing")
	}
}

// A render that fails leaves neither file: a timings file with no WAV beside
// it is a script's worst surprise.
func TestTimingsLeavesNothingWhenTheEngineFails(t *testing.T) {
	for name, engine := range map[string]*stubEngine{
		"down":  {healthy: false},
		"stale": {stale: true},
		"error": {healthy: true, err: errors.New("boom")},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
			dir := t.TempDir()
			out, timings := filepath.Join(dir, "a.wav"), filepath.Join(dir, "a.json")

			if err := s.Speak(context.Background(), "x", Options{Out: out, Timings: timings}); err == nil {
				t.Fatal("Speak succeeded")
			}
			for _, f := range []string{out, timings} {
				if _, err := os.Stat(f); !os.IsNotExist(err) {
					t.Errorf("%s exists after a failed render", f)
				}
			}
		})
	}
}

func TestSplitPatternNamesPresetsAndRejectsOthers(t *testing.T) {
	for name, want := range map[string]string{"": "", "line": "", "sentence": `(?<=[.!?])\s+`} {
		if got, err := SplitPattern(name); err != nil || got != want {
			t.Errorf("SplitPattern(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := SplitPattern("paragraph"); err == nil || !strings.Contains(err.Error(), "sentence") {
		t.Errorf("err = %v, want one listing line and sentence", err)
	}
}

// --- batch (OutDir) ------------------------------------------------------

// scene answers a line with audio named after it and one segment whose length
// is the line's length in ms, so a test can tell the lines apart.
func scene(text string) (mlx.TimedSpeech, error) {
	return mlx.TimedSpeech{
		Audio:    []byte("WAV:" + text),
		Segments: []captions.Segment{{Text: text, StartMS: 0, EndMS: len(text) * 100}},
	}, nil
}

type manifestEntry struct {
	Line       int                `json:"line"`
	File       string             `json:"file"`
	Text       string             `json:"text"`
	DurationMS int                `json:"duration_ms"`
	Segments   []captions.Segment `json:"segments"`
}

func readManifest(t *testing.T, dir string) []manifestEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m []manifestEntry
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest is not the documented JSON: %v\n%s", err, raw)
	}
	return m
}

// One WAV per non-blank line, numbered in order, and a manifest of what each
// holds. Blank lines are skipped but the manifest keeps each line's number in
// the script, so a scene can be traced back to where it was written.
func TestBatchWritesOneWavPerLineAndAManifest(t *testing.T) {
	engine := &stubEngine{healthy: true, timedFor: scene}
	s, rec := newTestSpeaker(t, engine, settingsWith(func(s *voiceconfig.Settings) { s.Muted = true }))
	dir := filepath.Join(t.TempDir(), "scenes")

	if err := s.Speak(context.Background(), "Once upon a time.\n\n  The end.  \r\n", Options{OutDir: dir, SplitPattern: "SPLIT"}); err != nil {
		t.Fatalf("Speak(OutDir) error = %v", err)
	}

	for file, want := range map[string]string{"001.wav": "WAV:Once upon a time.", "002.wav": "WAV:The end."} {
		if got, _ := os.ReadFile(filepath.Join(dir, file)); string(got) != want {
			t.Errorf("%s = %q, want %q", file, got, want)
		}
	}
	m := readManifest(t, dir)
	if len(m) != 2 {
		t.Fatalf("manifest has %d entries, want 2: %+v", len(m), m)
	}
	if m[0].Line != 1 || m[0].File != "001.wav" || m[0].Text != "Once upon a time." || m[0].DurationMS != 1700 {
		t.Errorf("entry 1 = %+v", m[0])
	}
	if m[1].Line != 3 || m[1].File != "002.wav" || m[1].Text != "The end." || m[1].DurationMS != 800 {
		t.Errorf("entry 2 = %+v, want it to remember it was line 3 of the script", m[1])
	}
	if len(m[0].Segments) != 1 || m[0].Segments[0].Text != "Once upon a time." {
		t.Errorf("entry 1 segments = %+v", m[0].Segments)
	}
	if engine.timedCalls[0].SplitPattern != "SPLIT" {
		t.Error("the split pattern was not passed to the engine")
	}
	if len(rec.played) != 0 || rec.saidText != "" || hasMarker(s.ActivityDir) {
		t.Error("batch made a sound or left a marker; it only writes files")
	}
}

// 1000 lines must still sort: the width grows past three digits.
func TestBatchNumbersWithAtLeastThreeDigits(t *testing.T) {
	if got := batchName(1, 9); got != "001.wav" {
		t.Errorf("batchName(1, 9) = %q", got)
	}
	if got := batchName(7, 12345); got != "00007.wav" {
		t.Errorf("batchName(7, 12345) = %q, want a width that fits every number", got)
	}
}

// A failure on line 2 writes nothing and says which line: half a scene set
// beside a manifest that is not there is worse than none.
func TestBatchWritesNothingWhenALineFails(t *testing.T) {
	engine := &stubEngine{healthy: true, timedFor: func(text string) (mlx.TimedSpeech, error) {
		if text == "bad" {
			return mlx.TimedSpeech{}, errors.New("boom")
		}
		return scene(text)
	}}
	s, _ := newTestSpeaker(t, engine, voiceconfig.Defaults())
	dir := filepath.Join(t.TempDir(), "scenes")

	err := s.Speak(context.Background(), "good\nbad\nalso good", Options{OutDir: dir})
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want it to name line 2", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("%d files written after a failure", len(entries))
	}
}

func TestBatchWithAnEngineThatIsDownOrStaleWritesNothing(t *testing.T) {
	for name, engine := range map[string]*stubEngine{
		"down":  {healthy: false},
		"stale": {stale: true},
	} {
		t.Run(name, func(t *testing.T) {
			s, rec := newTestSpeaker(t, engine, voiceconfig.Defaults())
			dir := filepath.Join(t.TempDir(), "scenes")

			if err := s.Speak(context.Background(), "one\ntwo", Options{OutDir: dir}); err == nil {
				t.Fatal("Speak succeeded")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("the directory exists after a failed batch: %v", err)
			}
			if rec.saidText != "" {
				t.Error("fell back to say")
			}
		})
	}
}

func TestBatchOfNothingIsAnError(t *testing.T) {
	s, _ := newTestSpeaker(t, &stubEngine{healthy: true, timedFor: scene}, voiceconfig.Defaults())
	err := s.Speak(context.Background(), " \n\n  ", Options{OutDir: filepath.Join(t.TempDir(), "x")})
	if err == nil || !strings.Contains(err.Error(), "nothing to speak") {
		t.Errorf("err = %v, want nothing to speak", err)
	}
}

func TestBatchAndSingleFileAreExclusive(t *testing.T) {
	s, _ := newTestSpeaker(t, &stubEngine{healthy: true, timedFor: scene}, voiceconfig.Defaults())
	for _, opts := range []Options{
		{OutDir: t.TempDir(), Out: "a.wav"},
		{OutDir: t.TempDir(), Timings: "a.json"},
	} {
		if err := s.Speak(context.Background(), "x", opts); err == nil {
			t.Errorf("Speak(%+v) succeeded; a batch writes its own files", opts)
		}
	}
}

// --- restart limit (RFC 0004 section 6) -----------------------------------

// guarded is a speaker whose auto-start can be made to fail, behind a start
// guard on a clock the test moves. calls counts the attempts.
func guarded(t *testing.T, engine *stubEngine) (s *Speaker, rec *recorder, c *clock, calls *int) {
	t.Helper()
	s, rec = newTestSpeaker(t, engine, voiceconfig.Defaults())
	c = &clock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	s.StartGuard = &startGuard{Path: filepath.Join(t.TempDir(), "failures"), Now: c.now, Max: 3, Window: 5 * time.Minute}
	n := 0
	calls = &n
	s.AutoStart = func(context.Context) error { n++; return errors.New("exit status 1: uvicorn crashed") }
	return s, rec, c, calls
}

func speakAndNotice(t *testing.T, s *Speaker) string {
	t.Helper()
	var notice strings.Builder
	s.Notice = &notice
	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatalf("Speak() error = %v: it should fall back to say, not fail", err)
	}
	return notice.String()
}

// An engine that crashes at startup is tried three times and then left alone:
// every speak, hooks included, used to pay for `uv sync` and a doomed start.
func TestAnEngineThatKeepsFailingToStartIsGivenUpOn(t *testing.T) {
	s, rec, c, calls := guarded(t, &stubEngine{healthy: false})
	// The notice names the time in the user's own zone.
	retry := c.t.Add(5 * time.Minute).Local().Format("15:04")

	var notices []string
	for i := 0; i < 5; i++ {
		notices = append(notices, speakAndNotice(t, s))
		if rec.saidText != "hello" {
			t.Fatalf("speak %d did not fall back to say", i+1)
		}
		rec.saidText = ""
	}
	if *calls != 3 {
		t.Errorf("auto-start was attempted %d times in 5 speaks, want 3", *calls)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(notices[i], "auto-start failed") {
			t.Errorf("speak %d notice %q, want the start failure", i+1, notices[i])
		}
	}
	for i := 3; i < 5; i++ {
		for _, want := range []string{"3 times", retry, "ava engine start", "to see why it fails"} {
			if !strings.Contains(notices[i], want) {
				t.Errorf("speak %d notice %q is missing %q", i+1, notices[i], want)
			}
		}
	}
}

func TestTheEngineIsTriedAgainOnceTheWindowHasPassed(t *testing.T) {
	s, _, c, calls := guarded(t, &stubEngine{healthy: false})
	for i := 0; i < 4; i++ {
		speakAndNotice(t, s)
	}
	if *calls != 3 {
		t.Fatalf("%d attempts before the window, want 3", *calls)
	}
	c.t = c.t.Add(5*time.Minute + time.Second)
	speakAndNotice(t, s)
	if *calls != 4 {
		t.Errorf("%d attempts after the window passed, want 4", *calls)
	}
}

// A start that returns cleanly but leaves the engine down is a failed start.
func TestAStartThatLeavesTheEngineDownCountsAsFailed(t *testing.T) {
	s, _, _, _ := guarded(t, &stubEngine{healthy: false})
	attempts := 0
	s.AutoStart = func(context.Context) error { attempts++; return nil } // "succeeds", engine stays down
	for i := 0; i < 5; i++ {
		speakAndNotice(t, s)
	}
	if attempts != 3 {
		t.Errorf("%d attempts, want 3", attempts)
	}
}

func TestASuccessfulStartClearsTheCount(t *testing.T) {
	engine := &stubEngine{healthy: false, audio: []byte("wav")}
	s, _, _, _ := guarded(t, engine)
	fail := true
	attempts := 0
	s.AutoStart = func(context.Context) error {
		attempts++
		if fail {
			return errors.New("crashed")
		}
		engine.healthy = true
		return nil
	}
	speakAndNotice(t, s)
	speakAndNotice(t, s) // two failures

	fail = false
	speakAndNotice(t, s) // the third attempt works and clears them
	if !engine.healthy {
		t.Fatal("the start did not bring the engine up")
	}

	engine.healthy = false // it dies again
	fail = true
	for i := 0; i < 5; i++ {
		speakAndNotice(t, s)
	}
	// 3 before, then 3 more: the success reset the count, so the earlier two
	// failures did not count towards the second round's limit.
	if attempts != 6 {
		t.Errorf("%d attempts in total, want 6 (3, a success, then 3 more)", attempts)
	}
}

// An engine the user started by hand is a success too: the failures before it
// should not hold against the next crash.
func TestAnEngineThatIsUpClearsTheCount(t *testing.T) {
	engine := &stubEngine{healthy: false}
	s, _, _, _ := guarded(t, engine)
	speakAndNotice(t, s)
	speakAndNotice(t, s)
	if _, err := os.Stat(s.StartGuard.Path); err != nil {
		t.Fatalf("no failures recorded: %v", err)
	}

	engine.healthy, engine.audio = true, []byte("wav")
	if err := s.Speak(context.Background(), "hello", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.StartGuard.Path); !os.IsNotExist(err) {
		t.Errorf("the failures were kept after the engine came up: %v", err)
	}
}

// Hooks are separate processes: a second speaker on the same file is limited
// by the first one's failures.
func TestAnotherProcessSeesTheFailures(t *testing.T) {
	first, _, c, _ := guarded(t, &stubEngine{healthy: false})
	for i := 0; i < 3; i++ {
		speakAndNotice(t, first)
	}

	second, _ := newTestSpeaker(t, &stubEngine{healthy: false}, voiceconfig.Defaults())
	second.StartGuard = &startGuard{Path: first.StartGuard.Path, Now: c.now, Max: 3, Window: 5 * time.Minute}
	attempts := 0
	second.AutoStart = func(context.Context) error { attempts++; return errors.New("crashed") }
	speakAndNotice(t, second)
	if attempts != 0 {
		t.Errorf("a second process started the engine %d times despite 3 recent failures", attempts)
	}
}

func TestOutIsRefusedWithTheSameReasonWhenTheLimitIsReached(t *testing.T) {
	s, _, _, _ := guarded(t, &stubEngine{healthy: false})
	for i := 0; i < 3; i++ {
		speakAndNotice(t, s)
	}
	out := filepath.Join(t.TempDir(), "a.wav")
	err := s.Speak(context.Background(), "hello", Options{Out: out})
	if err == nil || !strings.Contains(err.Error(), "3 times") {
		t.Fatalf("err = %v, want the limit named", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("a file was written")
	}
}

// A stale engine is not a failed start (nothing was started), and an operator
// who turned auto-start off has not failed to start anything either.
func TestOnlyFailedStartsCount(t *testing.T) {
	s, _, _, calls := guarded(t, &stubEngine{stale: true})
	for i := 0; i < 5; i++ {
		speakAndNotice(t, s)
	}
	if *calls != 0 {
		t.Errorf("auto-start ran %d times over a stale engine", *calls)
	}
	if _, err := os.Stat(s.StartGuard.Path); !os.IsNotExist(err) {
		t.Error("a stale engine was recorded as a failed start")
	}

	off, _ := newTestSpeaker(t, &stubEngine{healthy: false}, settingsWith(func(s *voiceconfig.Settings) { s.EngineAutoStart = false }))
	off.StartGuard = &startGuard{Path: filepath.Join(t.TempDir(), "failures"), Max: 3, Window: time.Minute}
	off.AutoStart = func(context.Context) error { t.Error("auto-start ran with it switched off"); return nil }
	speakAndNotice(t, off)
	if _, err := os.Stat(off.StartGuard.Path); !os.IsNotExist(err) {
		t.Error("a speak with auto-start off was recorded as a failed start")
	}
}

// The speaker the CLI, the MCP server and the hooks use has the limit; a guard
// only the tests install protects nobody.
func TestTheProductionSpeakerHasTheStartLimit(t *testing.T) {
	g := New("").StartGuard
	if g == nil {
		t.Fatal("New() has no start guard: a crashing engine would be restarted on every speak")
	}
	if g.Max != 3 || g.Window != 5*time.Minute {
		t.Errorf("guard = %d failures in %v, want 3 in 5m", g.Max, g.Window)
	}
	if !strings.HasPrefix(g.Path, os.TempDir()) {
		t.Errorf("guard file %q is not under the temp dir", g.Path)
	}
}
