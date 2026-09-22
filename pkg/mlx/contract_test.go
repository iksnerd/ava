package mlx

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mlx-engine's port is spelled in five places this repo controls: DefaultServerURL
// here, the server that binds it, the two things that start it, and the menu bar
// app that health-checks it. None of them can import a Go constant, so the copies
// stay and this test fails when they DISAGREE.
//
// It is not a style rule. Whichever process holds a stale port either fails to
// bind or health-checks a socket nobody is listening on, and the second one
// reports the engine as down while it is running.

func enginePort(t *testing.T) string {
	t.Helper()
	i := strings.LastIndex(DefaultServerURL, ":")
	if i < 0 {
		t.Fatalf("DefaultServerURL %q has no port", DefaultServerURL)
	}
	return DefaultServerURL[i+1:]
}

func mustRead(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestPortAgreesAcrossEveryRuntimeThatSpellsIt(t *testing.T) {
	port := enginePort(t)

	for _, c := range []struct {
		what    string
		body    string
		pattern string // one capture group: the port
	}{
		{"mlx-engine/server.py uvicorn.run", mustRead(t, "mlx-engine", "server.py"), `uvicorn\.run\([^)]*port=(\d+)`},
		{"mlx-engine/Makefile uvicorn", mustRead(t, "mlx-engine", "Makefile"), `--port\s+(\d+)`},
		{"scripts/mlx-engine-server.sh uvicorn", mustRead(t, "scripts", "mlx-engine-server.sh"), `--port\s+(\d+)`},
		{"scripts/mlx-engine-server.sh health probe", mustRead(t, "scripts", "mlx-engine-server.sh"), `127\.0\.0\.1:(\d+)/health`},
		{"ServerController.swift health probe", mustRead(t, "AvaMenuBar", "Sources", "AvaMenuBar", "ServerController.swift"), `127\.0\.0\.1:(\d+)/health`},
	} {
		m := regexp.MustCompile(c.pattern).FindStringSubmatch(c.body)
		if m == nil {
			t.Errorf("%s: found no port matching %s — if that line was rewritten, "+
				"update this pattern rather than deleting the check", c.what, c.pattern)
			continue
		}
		if m[1] != port {
			t.Errorf("%s uses port %s, pkg/mlx.DefaultServerURL uses %s — one of them "+
				"is talking to a socket nobody is listening on", c.what, m[1], port)
		}
	}
}

// A port quoted in the docs that appears nowhere in the source is stale or
// invented — the failure mode when a service moves and its prose does not.
// The invariant is deliberately weak: it cannot tell which service a sentence
// means, so it only asks that the number exist somewhere in the code. An
// earlier, stronger version asserted every documented port was one this repo
// *binds*, and failed twice on correct docs — once on voice-monitor's port,
// once on Ollama's, which this project talks to but does not serve.
func TestEveryPortInTheDocsExistsInTheSource(t *testing.T) {
	root := filepath.Join("..", "..")

	inCode := map[string]bool{}
	portRe := regexp.MustCompile(`(?:127\.0\.0\.1|localhost):(\d+)|--port\s+(\d+)|port=(\d+)`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".venv", "node_modules", "bin", ".build":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(name) {
		case ".go", ".py", ".sh", ".swift":
		default:
			if name != "Makefile" {
				return nil
			}
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range portRe.FindAllStringSubmatch(string(body), -1) {
			for _, g := range m[1:] {
				if g != "" {
					inCode[g] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if !inCode[enginePort(t)] {
		t.Fatalf("the walk found no ports at all (engine port %s missing) — "+
			"the extensions or patterns above stopped matching", enginePort(t))
	}

	docRe := regexp.MustCompile(`(?:127\.0\.0\.1|localhost):(\d+)`)
	for _, doc := range [][]string{{"SECURITY.md"}, {"README.md"}, {"docs", "architecture.md"}, {"docs", "mcp.md"}, {"docs", "voice-monitor.md"}, {"docs", "troubleshooting.md"}} {
		for _, m := range docRe.FindAllStringSubmatch(mustRead(t, doc...), -1) {
			if !inCode[m[1]] {
				t.Errorf("%s quotes port %s, which appears in no source file — stale doc",
					filepath.Join(doc...), m[1])
			}
		}
	}
}
