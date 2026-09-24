package voiceconfig

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This file is the drift guard for a fact that isn't visible from any one
// language: three separate readers resolve the same config file, and they
// have to agree. scripts/lib.sh reads it in bash for the voice hooks, this
// package reads it in Go for the CLI and the MCP server, and
// AvaMenuBar/VoiceSettings.swift reads it in Swift for the UI.
//
// Bash reads only bashKeys. It resolved the speech settings too until
// speak.sh became a hand-off to `ava speak`, and a reader that no longer
// exists has nothing to agree with.
//
// Pinning the default *values* (TestDefaultsMatchVoiceDefaultsJSON) is not
// enough: what actually diverges is the resolution *logic* — what each
// reader does with a key that's missing, wrongly typed, or in a file that
// won't parse. So the cases live in testdata/voice-config-cases.json, in no
// language in particular, and every reader is run against them.
//
// Swift is not covered here: the menu bar app has no test target, and
// putting swiftc on the default test path would break `make test` on a
// machine without Xcode. It has its own arm against these same fixtures,
// `make check-swift-config`, which extracts VoiceConfig's decoder verbatim
// from VoiceSettings.swift rather than copying it. Fold it in here if that
// target ever appears; the fixture file is language-neutral on purpose.

type contractCase struct {
	Name   string            `json:"name"`
	Live   map[string]any    `json:"live"`
	Raw    string            `json:"raw"`
	Expect map[string]string `json:"expect"`
}

// keyKind decides how a resolved value is compared, and which of bash's
// accessors is the right one to call. Each kind must name the accessor the
// real call sites use: raw config_get returns whatever is in the file, while
// config_get_bool/_int coerce. Testing an accessor no caller uses
// proves nothing.
var keyKind = map[string]string{
	"muted":           "bool",
	"engineAutoStart": "bool",
	"speed":           "float",
	"volume":          "float",
	"sayRate":         "int",
	"voice":           "string",
}

func loadContractCases(t *testing.T) []contractCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "voice-config-cases.json"))
	if err != nil {
		t.Fatalf("read contract fixtures: %v", err)
	}
	var file struct {
		Cases []contractCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parse contract fixtures: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("no cases in testdata/voice-config-cases.json")
	}
	return file.Cases
}

// defaultsFromJSON reads scripts/voice-defaults.json as raw strings, so
// "default" in a fixture resolves to whatever that file actually says
// rather than to a number repeated in this test.
func defaultsFromJSON(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "voice-defaults.json"))
	if err != nil {
		t.Fatalf("read voice-defaults.json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse voice-defaults.json: %v", err)
	}

	out := map[string]string{}
	for key, value := range raw {
		switch v := value.(type) {
		case bool:
			out[key] = strconv.FormatBool(v)
		case float64:
			out[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case string:
			out[key] = v
		}
	}
	return out
}

// writeCaseConfig materializes a case's live config and returns its path.
// A case with neither `live` nor `raw` means the file does not exist.
func writeCaseConfig(t *testing.T, c contractCase) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")

	switch {
	case c.Raw != "":
		if err := os.WriteFile(path, []byte(c.Raw), 0644); err != nil {
			t.Fatalf("write raw config: %v", err)
		}
	case c.Live != nil:
		data, err := json.Marshal(c.Live)
		if err != nil {
			t.Fatalf("marshal live config: %v", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write live config: %v", err)
		}
	}
	return path
}

func goResolved(t *testing.T, configPath, key string) string {
	t.Helper()
	t.Setenv(ConfigPathEnv, configPath)
	s := Load()

	switch key {
	case "muted":
		return strconv.FormatBool(s.Muted)
	case "engineAutoStart":
		return strconv.FormatBool(s.EngineAutoStart)
	case "speed":
		return strconv.FormatFloat(s.Speed, 'f', -1, 64)
	case "volume":
		return strconv.FormatFloat(s.Volume, 'f', -1, 64)
	case "sayRate":
		return strconv.Itoa(s.SayRate)
	case "voice":
		return s.Voice
	}
	t.Fatalf("no Go accessor for key %q", key)
	return ""
}

// bashKeys are the keys scripts/lib.sh reads: the mute every hook checks
// and the engine auto-start mlx-engine-server.sh honours.
var bashKeys = map[string]bool{"muted": true, "engineAutoStart": true}

// bashResolved calls the accessor scripts/lib.sh actually offers for this
// key, with the same last-resort default its real call sites pass.
func bashResolved(t *testing.T, configPath, key, lastResort string) string {
	t.Helper()

	var call string
	switch keyKind[key] {
	case "bool":
		call = fmt.Sprintf("config_get_bool %s %s", key, lastResort)
	case "int":
		call = fmt.Sprintf("config_get_int %s %s", key, lastResort)
	default:
		call = fmt.Sprintf("config_get %s", key)
	}

	libPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "lib.sh"))
	if err != nil {
		t.Fatalf("resolve lib.sh: %v", err)
	}

	cmd := exec.Command("bash", "-c", fmt.Sprintf("source %q; %s", libPath, call))
	cmd.Env = append(os.Environ(), "VOICE_CONFIG_FILE="+configPath)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash %s: %v", call, err)
	}
	return strings.TrimSpace(string(out))
}

// equivalent compares two resolved values the way the key's type means
// them: 1 and 1.0 are the same speed even though the strings differ.
func equivalent(key, a, b string) bool {
	switch keyKind[key] {
	case "float":
		x, errA := strconv.ParseFloat(a, 64)
		y, errB := strconv.ParseFloat(b, 64)
		if errA != nil || errB != nil {
			return false
		}
		return math.Abs(x-y) < 1e-9
	case "int":
		x, errA := strconv.Atoi(a)
		y, errB := strconv.Atoi(b)
		if errA != nil || errB != nil {
			return false
		}
		return x == y
	default:
		return a == b
	}
}

func TestGoMatchesTheContract(t *testing.T) {
	defaults := defaultsFromJSON(t)

	for _, c := range loadContractCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			configPath := writeCaseConfig(t, c)

			for key, want := range c.Expect {
				if want == "default" {
					want = defaults[key]
				}
				if got := goResolved(t, configPath, key); !equivalent(key, got, want) {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

// The point of the whole file: bash and Go must answer identically, not
// merely each match its own idea of correct.
func TestBashAndGoAgree(t *testing.T) {
	defaults := defaultsFromJSON(t)

	for _, c := range loadContractCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			configPath := writeCaseConfig(t, c)

			for key, want := range c.Expect {
				if !bashKeys[key] {
					continue
				}
				if want == "default" {
					want = defaults[key]
				}

				goValue := goResolved(t, configPath, key)
				bashValue := bashResolved(t, configPath, key, defaults[key])

				if !equivalent(key, goValue, bashValue) {
					t.Errorf("%s: bash says %q, Go says %q (contract wants %q) — the two readers of this config have diverged",
						key, bashValue, goValue, want)
				}
			}
		})
	}
}
