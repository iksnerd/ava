package mlx

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mlx-engine's port used to be spelled in five places. Four of them now derive
// it from internal/protocol/protocol.json — server.py and the start script read
// the generated ENGINE_URL, the menu bar app reads the generated Swift constant.
//
// One cannot: mlx-engine/Makefile passes --port to uvicorn, and make cannot
// source a shell file for one word without more machinery than the line is
// worth. That is the honest shape of generation — it covers most consumers,
// not all — so the leftover stays pinned here, and this test fails when it
// DISAGREES with the Go constant rather than when it merely looks wrong.

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

func TestUngeneratedPortSpellingAgreesWithTheConstant(t *testing.T) {
	port := enginePort(t)
	body := mustRead(t, "mlx-engine", "Makefile")

	m := regexp.MustCompile(`--port\s+(\d+)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("mlx-engine/Makefile no longer passes --port to uvicorn — if it " +
			"now reads the generated value, delete this test rather than the check")
	}
	if m[1] != port {
		t.Errorf("mlx-engine/Makefile starts uvicorn on port %s, pkg/mlx.DefaultServerURL "+
			"is %s — `make -C mlx-engine` would bind where no client is looking", m[1], port)
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
