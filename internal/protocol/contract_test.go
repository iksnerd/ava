package protocol

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// With the constants generated, the old question — do the copies agree? — is
// answered by construction. The question that replaces it is whether a
// hand-written copy has appeared *beside* the generated one, because a
// generator only guarantees its own output is right. It cannot make anyone
// read it, and a literal sitting next to a generated constant is the same bug
// as before, now wearing a reassuring file.
//
// So: every protocol value must appear only in files that are generated from
// protocol.json, in protocol.json itself, or in a test. Anywhere else is a
// copy that will drift.

// generated lists the rendered outputs, plus the source and the generator.
// A path is exempt because it is derived, not because it is convenient.
var generated = map[string]bool{
	"internal/protocol/protocol.json":              true,
	"internal/protocol/protocol_gen.go":            true,
	"internal/protocol/gen/main.go":                true,
	"scripts/protocol.sh":                          true,
	"AvaMenuBar/Sources/AvaMenuBar/Protocol.swift": true,
	"mlx-engine/protocol.py":                       true,
}

// knownProse are files that legitimately quote a value at a human. A doc
// naming /tmp/ava-tts-active is describing the system, not implementing it.
func isProse(rel string) bool {
	switch filepath.Ext(rel) {
	case ".md", ".mmd", ".json":
		return true
	}
	return false
}

func TestNoHandWrittenCopyOfAnyProtocolValue(t *testing.T) {
	root := filepath.Join("..", "..")

	raw, err := os.ReadFile(filepath.Join(root, "internal", "protocol", "protocol.json"))
	if err != nil {
		t.Fatalf("read protocol.json: %v", err)
	}
	// Into RawMessage first: the "_comment" key is an array, and a typed
	// unmarshal fails on it before the underscore skip below can run.
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatalf("parse protocol.json: %v", err)
	}

	values := map[string]string{} // value -> key
	for k, rawEntry := range all {
		if strings.HasPrefix(k, "_") {
			continue
		}
		var v struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(rawEntry, &v); err != nil || v.Value == "" {
			continue
		}
		// Suffixes like ".stopped" are too short to scan for without drowning
		// in false positives; the sidecar names are covered by the stop-path
		// test in internal/ttscontrol instead.
		if len(v.Value) < 12 {
			continue
		}
		values[v.Value] = k
	}
	if len(values) == 0 {
		t.Fatal("no values long enough to scan — protocol.json changed shape")
	}

	var checked int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".venv", "node_modules", "bin", ".build", "__pycache__", ".ruff_cache", ".pytest_cache":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(d.Name()) {
		case ".go", ".py", ".sh", ".swift":
		default:
			if d.Name() != "Makefile" {
				return nil
			}
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if generated[rel] || isProse(rel) || strings.HasSuffix(rel, "_test.go") ||
			strings.Contains(rel, "/tests/") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		checked++
		for _, line := range strings.Split(string(body), "\n") {
			code := line
			// A value named in a comment is documentation, not a copy.
			for _, marker := range []string{"//", "#"} {
				if i := strings.Index(code, marker); i >= 0 {
					code = code[:i]
				}
			}
			for value, key := range values {
				if strings.Contains(code, value) {
					t.Errorf("%s spells %q by hand; it is generated as %q from "+
						"internal/protocol/protocol.json. Read the generated constant, "+
						"or add this file to `generated` if it is rendered too.",
						rel, value, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatal("scanned no files — the extension filter above stopped matching, " +
			"and this test would pass vacuously forever")
	}
	t.Logf("scanned %d source files for %d protocol values", checked, len(values))
}
