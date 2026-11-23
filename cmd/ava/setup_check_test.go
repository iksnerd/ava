package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/internal/testutil"
)

// setupRig is a machine for `ava setup --check` to look at: stub tools on
// PATH, a home for the whisper model, an engine dir, and a brew that records
// whether anything tried to install.
type setupRig struct {
	bin, home, engine, brewCalls string
}

func newSetupRig(t *testing.T) *setupRig {
	t.Helper()
	r := &setupRig{bin: t.TempDir(), home: t.TempDir(), engine: shortEngineDir(t)}
	r.brewCalls = filepath.Join(t.TempDir(), "brew-calls")
	testutil.WriteExec(t, filepath.Join(r.bin, "brew"), "#!/bin/sh\necho \"$@\" >> "+r.brewCalls+"\n")
	testutil.SetPath(t, r.bin, "/usr/bin", "/bin")
	t.Setenv("HOME", r.home)
	// Never ask whatever is really listening on 8765 on the machine running the tests.
	setupEngineURL = "http://127.0.0.1:1"
	t.Cleanup(func() { setupEngineURL = "" })
	return r
}

func (r *setupRig) addTools(t *testing.T) {
	for bin := range setupToolFormulae {
		testutil.WriteExec(t, filepath.Join(r.bin, bin), "#!/bin/sh\nexit 0\n")
	}
}

// addModel writes a sparse file of the base model's exact size, which is
// what setup's own skip rule looks at.
func (r *setupRig) addModel(t *testing.T) {
	m := whisperModels["base"]
	dir := filepath.Join(r.home, modelDirRel)
	os.MkdirAll(dir, 0755)
	f, err := os.Create(filepath.Join(dir, m.file))
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(m.size)
	f.Close()
}

// addEngine installs a stub bundle whose control script answers `fetch
// --check` with $STUB_MODEL_MISSING (0 = the Kokoro model is there).
func (r *setupRig) addEngine(t *testing.T) {
	venv := filepath.Join(r.engine, "mlx-engine", ".venv", "bin")
	os.MkdirAll(venv, 0755)
	testutil.WriteExec(t, filepath.Join(venv, "uvicorn"), "#!/bin/sh\n")
	script := filepath.Join(r.engine, enginedist.ScriptRelPath)
	os.MkdirAll(filepath.Dir(script), 0755)
	testutil.WriteExec(t, script, `#!/bin/sh
[ "$1 $2" = "fetch --check" ] || { echo "unexpected: $*" >&2; exit 9; }
exit "${STUB_MODEL_MISSING:-0}"
`)
}

func (r *setupRig) check(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"setup", "--check"}, args...))
	err := root.Execute()
	return out.String(), err
}

func (r *setupRig) assertNothingInstalled(t *testing.T) {
	t.Helper()
	if calls, _ := os.ReadFile(r.brewCalls); len(calls) > 0 {
		t.Errorf("--check ran brew: %s", calls)
	}
	if _, err := os.Stat(filepath.Join(r.home, modelDirRel)); err == nil {
		t.Error("--check created the model directory")
	}
}

func skipOffAppleSilicon(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("the engine steps only exist on Apple Silicon")
	}
}

// The menu bar app runs this at every start to decide whether to offer Set
// up, so it has to agree with what `ava setup` would do, and never do it.
func TestSetupCheckPassesWhenEverythingIsThere(t *testing.T) {
	skipOffAppleSilicon(t)
	r := newSetupRig(t)
	r.addTools(t)
	r.addModel(t)
	r.addEngine(t)

	out, err := r.check(t)
	if err != nil {
		t.Fatalf("setup --check on a set-up machine: %v\n%s", err, out)
	}
	if !strings.Contains(out, "set up") {
		t.Errorf("output %q does not say Ava is set up", out)
	}
}

func TestSetupCheckNamesWhatIsMissingAndInstallsNothing(t *testing.T) {
	skipOffAppleSilicon(t)
	r := newSetupRig(t)

	out, err := r.check(t)
	if err == nil {
		t.Fatalf("setup --check on a bare machine passed:\n%s", out)
	}
	for _, want := range []string{"sox", "whisper-cli", "uv", "speech model", "Kokoro engine", "Kokoro model"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not name %q:\n%s", want, out)
		}
	}
	r.assertNothingInstalled(t)
}

// The model lives in the Hugging Face cache at a revision only the engine
// knows, so the check asks the engine instead of guessing.
func TestSetupCheckAsksTheEngineAboutItsModel(t *testing.T) {
	skipOffAppleSilicon(t)
	r := newSetupRig(t)
	r.addTools(t)
	r.addModel(t)
	r.addEngine(t)
	t.Setenv("STUB_MODEL_MISSING", "1")

	out, err := r.check(t)
	if err == nil {
		t.Fatalf("setup --check passed with the Kokoro model missing:\n%s", out)
	}
	if !strings.Contains(out, "Kokoro model") || strings.Contains(out, "Kokoro engine") {
		t.Errorf("want only the Kokoro model named as missing:\n%s", out)
	}
}

func TestSetupCheckWithSkipEngineIgnoresTheEngine(t *testing.T) {
	r := newSetupRig(t)
	r.addTools(t)
	r.addModel(t)

	if out, err := r.check(t, "--skip-engine"); err != nil {
		t.Fatalf("setup --check --skip-engine with dictation ready: %v\n%s", err, out)
	}
}
