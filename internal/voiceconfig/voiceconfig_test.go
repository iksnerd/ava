package voiceconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeLiveConfig points Load() at a throwaway config file holding cfg.
func writeLiveConfig(t *testing.T, cfg map[string]any) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(ConfigPathEnv, path)
}

func TestLoadFallsBackToDefaultsWithoutLiveConfig(t *testing.T) {
	t.Setenv(ConfigPathEnv, filepath.Join(t.TempDir(), "does-not-exist.json"))

	got := Load()
	if got != Defaults() {
		t.Errorf("Load() = %+v, want the defaults %+v", got, Defaults())
	}
}

func TestLoadLiveConfigWins(t *testing.T) {
	writeLiveConfig(t, map[string]any{"muted": true, "voice": "bm_george", "speed": 0.9})

	got := Load()
	if !got.Muted {
		t.Error("Muted = false, want true from the live config")
	}
	if got.Voice != "bm_george" {
		t.Errorf("Voice = %q, want %q", got.Voice, "bm_george")
	}
	if got.Speed != 0.9 {
		t.Errorf("Speed = %v, want 0.9", got.Speed)
	}
}

// lib.sh's config_get falls back per key, not per file: a live config that
// only sets `muted` must still get every other value from the defaults.
func TestLoadFallsBackPerKey(t *testing.T) {
	writeLiveConfig(t, map[string]any{"muted": true})

	got := Load()
	if got.Voice != Defaults().Voice {
		t.Errorf("Voice = %q, want the default %q", got.Voice, Defaults().Voice)
	}
	if got.Speed != Defaults().Speed {
		t.Errorf("Speed = %v, want the default %v", got.Speed, Defaults().Speed)
	}
	if got.SayRate != Defaults().SayRate {
		t.Errorf("SayRate = %v, want the default %v", got.SayRate, Defaults().SayRate)
	}
}

func TestLoadMalformedLiveConfigFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(ConfigPathEnv, path)

	if got := Load(); got != Defaults() {
		t.Errorf("Load() = %+v, want the defaults for an unparseable config", got)
	}
}

// A hand-edited config can hold the wrong type for a key. lib.sh's
// config_get_int/config_get_bool degrade to the default rather than erroring;
// so should this.
func TestLoadIgnoresWrongTypedValues(t *testing.T) {
	writeLiveConfig(t, map[string]any{"speed": "fast", "sayRate": map[string]any{}, "muted": "yes"})

	got := Load()
	if got.Speed != Defaults().Speed {
		t.Errorf("Speed = %v, want the default %v", got.Speed, Defaults().Speed)
	}
	if got.SayRate != Defaults().SayRate {
		t.Errorf("SayRate = %v, want the default %v", got.SayRate, Defaults().SayRate)
	}
	if got.Muted {
		t.Error("Muted = true, want the default false for a non-bool value")
	}
}

// TTS_SPEED/TTS_VOLUME/TTS_SAY_RATE beat both files, matching lib.sh's
// `SPEED="${TTS_SPEED:-$(config_get speed)}"`.
func TestEnvOverridesBeatLiveConfig(t *testing.T) {
	writeLiveConfig(t, map[string]any{"speed": 0.9, "volume": 0.2, "sayRate": 150})
	t.Setenv("TTS_SPEED", "1.75")
	t.Setenv("TTS_VOLUME", "0.5")
	t.Setenv("TTS_SAY_RATE", "300")

	got := Load()
	if got.Speed != 1.75 {
		t.Errorf("Speed = %v, want 1.75 from TTS_SPEED", got.Speed)
	}
	if got.Volume != 0.5 {
		t.Errorf("Volume = %v, want 0.5 from TTS_VOLUME", got.Volume)
	}
	if got.SayRate != 300 {
		t.Errorf("SayRate = %v, want 300 from TTS_SAY_RATE", got.SayRate)
	}
}

func TestUnparseableEnvOverrideIsIgnored(t *testing.T) {
	t.Setenv(ConfigPathEnv, filepath.Join(t.TempDir(), "none.json"))
	t.Setenv("TTS_SPEED", "very fast")

	if got := Load().Speed; got != Defaults().Speed {
		t.Errorf("Speed = %v, want the default %v for an unparseable TTS_SPEED", got, Defaults().Speed)
	}
}

// Defaults() is hand-maintained Go rather than an embedded copy of
// scripts/voice-defaults.json (go:embed can't reach a parent directory, and
// a second copy of the file would defeat its "single source of truth" job).
// This test is what keeps the two honest.
func TestDefaultsMatchVoiceDefaultsJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "voice-defaults.json"))
	if err != nil {
		t.Fatalf("read scripts/voice-defaults.json: %v", err)
	}
	var file struct {
		Muted           bool    `json:"muted"`
		Speed           float64 `json:"speed"`
		Volume          float64 `json:"volume"`
		Voice           string  `json:"voice"`
		SayRate         int     `json:"sayRate"`
		EngineAutoStart bool    `json:"engineAutoStart"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parse scripts/voice-defaults.json: %v", err)
	}

	want := Settings{
		Muted:           file.Muted,
		Speed:           file.Speed,
		Volume:          file.Volume,
		Voice:           file.Voice,
		SayRate:         file.SayRate,
		EngineAutoStart: file.EngineAutoStart,
	}
	if Defaults() != want {
		t.Errorf("Defaults() = %+v, but scripts/voice-defaults.json says %+v — update Defaults() to match", Defaults(), want)
	}
}
