// Package voiceconfig reads the Ava settings that the menu bar app
// writes and scripts/speak.sh reads — global mute, TTS speed/volume/voice,
// the `say` fallback rate, and whether the mlx-engine may auto-start.
//
// This is a Go port of scripts/lib.sh's config_get/config_get_bool helpers,
// not a wrapper around them: local-whisper is typically installed to
// ~/.local/bin standalone (see `make install-bin`), so it can't assume the
// repo's scripts/ directory is reachable — the same constraint
// internal/ttscontrol documents. Keep the two in sync by hand if the
// resolution rules change; TestDefaultsMatchVoiceDefaultsJSON keeps the
// default *values* in sync automatically.
//
// Resolution order, matching lib.sh exactly:
//
//	TTS_SPEED / TTS_VOLUME / TTS_SAY_RATE  (env, these three keys only)
//	~/Library/Application Support/ava/config.json  (live, per key)
//	Defaults()  (scripts/voice-defaults.json's values)
//
// Anything missing, unparseable or of the wrong type falls through to the
// next source rather than erroring: a hand-edited config should degrade to
// silence-free defaults, never stop a hook from speaking.
package voiceconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// ConfigPathEnv overrides the live config's location. Its only real use is
// pointing tests at an isolated file, since they can't otherwise exercise
// Load() without reading (and depending on) the real settings of whoever is
// running the test.
const ConfigPathEnv = "VOICECONFIG_PATH"

// Settings is the resolved configuration. Comparable on purpose, so tests
// and callers can check a whole resolution against Defaults() in one go.
type Settings struct {
	// Muted is the menu bar app's global mute switch — the one choke point
	// every speak path checks before synthesizing anything.
	Muted bool
	// Speed is Kokoro's speed multiplier.
	Speed float64
	// Volume is afplay's -v, 0.0-1.0+.
	Volume float64
	// Voice is a Kokoro voice id, or a comma-separated pair to blend.
	Voice string
	// SayRate is words/min for the macOS `say` fallback.
	SayRate int
	// EngineAutoStart is false once the user has explicitly stopped the
	// server, so an on-demand speak doesn't revive what they just killed.
	EngineAutoStart bool
}

// Defaults mirrors scripts/voice-defaults.json, which
// AvaMenuBar/VoiceSettings.swift and scripts/lib.sh also read.
// go:embed can't reach a parent directory and a second copy of the file
// would defeat its single-source-of-truth job, so these are hand-maintained
// and pinned by TestDefaultsMatchVoiceDefaultsJSON.
func Defaults() Settings {
	return Settings{
		Muted:           false,
		Speed:           1.3,
		Volume:          1.0,
		Voice:           "af_heart",
		SayRate:         220,
		EngineAutoStart: true,
	}
}

// ConfigPath is where AvaMenuBar persists live settings.
func ConfigPath() string {
	if v := os.Getenv(ConfigPathEnv); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "ava", "config.json")
}

// Load resolves the current settings. It never fails: every unreadable or
// nonsensical source is skipped in favour of the next one down.
func Load() Settings {
	s := Defaults()
	applyLiveConfig(&s, ConfigPath())
	applyEnvOverrides(&s)
	return s
}

// Muted is the common case — "may I speak at all" — without the caller
// having to remember which field gates it.
func Muted() bool { return Load().Muted }

func applyLiveConfig(s *Settings, path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var live map[string]any
	if err := json.Unmarshal(data, &live); err != nil {
		return
	}

	if v, ok := live["muted"].(bool); ok {
		s.Muted = v
	}
	if v, ok := live["speed"].(float64); ok {
		s.Speed = v
	}
	if v, ok := live["volume"].(float64); ok {
		s.Volume = v
	}
	if v, ok := live["voice"].(string); ok && v != "" {
		s.Voice = v
	}
	// JSON numbers decode as float64 even when the file wrote an integer.
	if v, ok := live["sayRate"].(float64); ok {
		s.SayRate = int(v)
	}
	if v, ok := live["engineAutoStart"].(bool); ok {
		s.EngineAutoStart = v
	}
}

func applyEnvOverrides(s *Settings) {
	if v, ok := envFloat("TTS_SPEED"); ok {
		s.Speed = v
	}
	if v, ok := envFloat("TTS_VOLUME"); ok {
		s.Volume = v
	}
	if v, ok := envFloat("TTS_SAY_RATE"); ok {
		s.SayRate = int(v)
	}
}

func envFloat(name string) (float64, bool) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
