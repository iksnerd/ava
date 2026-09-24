package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// syscallZero probes whether a process exists without signalling it.
var syscallZero = syscall.Signal(0)

// These run scripts/mlx-engine-server.sh as a process, the way `ava engine`,
// speak.sh and the menu bar app do, against throwaway layouts. The
// AVA_ENGINE_* overrides keep them off the real pid file, log, lock and port.

// stubUvicorn is a stand-in for the venv's uvicorn: a Python script like the
// real one, so its process command line looks the same. It records how it was
// started, then serves /health, or exits at once if STUB_UVICORN_FAIL is set.
const stubUvicorn = `#!/usr/bin/env python3
import http.server, os, sys
out = os.environ["STUB_OUT"]
open(os.path.join(out, "env"), "w").write("\n".join(f"{k}={v}" for k, v in os.environ.items()))
open(os.path.join(out, "argv"), "w").write(" ".join(sys.argv[1:]))
open(os.path.join(out, "cwd"), "w").write(os.getcwd())
if os.environ.get("STUB_UVICORN_FAIL"):
    print("stub uvicorn: failing on purpose", file=sys.stderr)
    sys.exit(1)
port = int(sys.argv[sys.argv.index("--port") + 1])
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200 if self.path == "/health" else 404)
        self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
`

// engineLayout is one place mlx-engine can live: <root>/mlx-engine with a
// venv, as in a checkout or the bundle `ava setup` installs.
func engineLayout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "mlx-engine", ".venv", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "uvicorn"), []byte(stubUvicorn), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

// copyScripts puts the real control script and what it sources under dir.
func copyScripts(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mlx-engine-server.sh", "lib.sh", "protocol.sh", "voice-defaults.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "mlx-engine-server.sh")
}

type engineRun struct {
	t      *testing.T
	env    []string
	stub   string // where the stub uvicorn writes what it saw
	config string
	url    string
	pid    string
}

func newEngineRun(t *testing.T) *engineRun {
	t.Helper()
	tmp := t.TempDir()
	stubBin := filepath.Join(tmp, "bin")
	os.MkdirAll(stubBin, 0755)
	// `uv sync` in a stub layout has nothing to sync.
	os.WriteFile(filepath.Join(stubBin, "uv"), []byte("#!/bin/sh\nexit 0\n"), 0755)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	r := &engineRun{
		t:      t,
		stub:   filepath.Join(tmp, "stub"),
		config: filepath.Join(tmp, "config.json"),
		url:    fmt.Sprintf("http://127.0.0.1:%d", port),
		pid:    filepath.Join(tmp, "engine.pid"),
	}
	os.MkdirAll(r.stub, 0755)
	os.WriteFile(r.config, []byte(`{"engineAutoStart": true}`), 0644)
	r.env = append(os.Environ(),
		"PATH="+stubBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AVA_ENGINE_PID_FILE="+r.pid,
		"AVA_ENGINE_LOG="+filepath.Join(tmp, "engine.log"),
		"AVA_ENGINE_LOCKDIR="+filepath.Join(tmp, "start.lockdir"),
		"AVA_ENGINE_URL="+r.url,
		"VOICE_CONFIG_FILE="+r.config,
		"STUB_OUT="+r.stub,
		// Never fall through to a real installed engine.
		"AVA_ENGINE_DIR="+filepath.Join(tmp, "no-installed-engine"),
	)
	t.Cleanup(func() { r.script(copyScripts(t, filepath.Join(tmp, "cleanup")), "stop", "--keep-autostart") })
	return r
}

func (r *engineRun) script(path string, args ...string) (string, error) {
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *engineRun) saw(what string) string {
	data, _ := os.ReadFile(filepath.Join(r.stub, what))
	return string(data)
}

func (r *engineRun) healthy() bool {
	resp, err := http.Get(r.url + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func TestEngineStartsFromACheckout(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	script := copyScripts(t, filepath.Join(checkout, "scripts"))

	if out, err := r.script(script, "start"); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	if !r.healthy() {
		t.Fatal("start returned but the server is not answering /health")
	}
	if cwd := r.saw("cwd"); !strings.HasSuffix(cwd, filepath.Join("mlx-engine")) || !strings.Contains(cwd, filepath.Base(checkout)) {
		t.Errorf("server ran in %q, want %s/mlx-engine", cwd, checkout)
	}
	// The server removes this file on idle exit, so it has to receive it.
	if env := r.saw("env"); !strings.Contains(env, "MLX_ENGINE_PID_FILE="+r.pid) {
		t.Error("the server did not receive MLX_ENGINE_PID_FILE")
	}
	if argv := r.saw("argv"); !strings.Contains(argv, "server:app --host 127.0.0.1 --port") {
		t.Errorf("server argv = %q, want it bound to 127.0.0.1", argv)
	}
}

// The menu bar app runs a copy of scripts/ from its Resources/, with no
// mlx-engine/ beside it; build-app.sh records the checkout in engine-root.
func TestEngineStartsFromTheAppBundleViaEngineRoot(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	resources := filepath.Join(t.TempDir(), "Ava.app", "Contents", "Resources")
	script := copyScripts(t, filepath.Join(resources, "scripts"))
	os.WriteFile(filepath.Join(resources, "engine-root"), []byte(checkout+"\n"), 0644)

	if out, err := r.script(script, "start"); err != nil {
		t.Fatalf("start from the app bundle: %v\n%s", err, out)
	}
	if !strings.Contains(r.saw("cwd"), filepath.Base(checkout)) {
		t.Errorf("server ran in %q, want the recorded checkout %s", r.saw("cwd"), checkout)
	}
}

func TestEngineStartsFromTheInstalledBundle(t *testing.T) {
	r := newEngineRun(t)
	installed := engineLayout(t)
	script := copyScripts(t, filepath.Join(t.TempDir(), "scripts"))
	r.env = append(r.env, "AVA_ENGINE_DIR="+installed)

	if out, err := r.script(script, "start"); err != nil {
		t.Fatalf("start with only an installed bundle: %v\n%s", err, out)
	}
	if !strings.Contains(r.saw("cwd"), filepath.Base(installed)) {
		t.Errorf("server ran in %q, want the installed bundle %s", r.saw("cwd"), installed)
	}
}

func TestEngineStartFailsFastWithNoEngine(t *testing.T) {
	r := newEngineRun(t)
	script := copyScripts(t, filepath.Join(t.TempDir(), "scripts"))

	start := time.Now()
	out, err := r.script(script, "start")
	if err == nil {
		t.Fatalf("start succeeded with no engine anywhere:\n%s", out)
	}
	if time.Since(start) > 5*time.Second || !strings.Contains(out, "ava setup") {
		t.Errorf("took %s and said %q; want a prompt failure naming `ava setup`", time.Since(start), out)
	}
}

func TestEngineStartReportsAServerThatDiesImmediately(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	script := copyScripts(t, filepath.Join(checkout, "scripts"))
	r.env = append(r.env, "STUB_UVICORN_FAIL=1")

	start := time.Now()
	out, err := r.script(script, "start")
	if err == nil {
		t.Fatalf("start succeeded although the server exited:\n%s", out)
	}
	if time.Since(start) > 10*time.Second || !strings.Contains(out, "exited during startup") {
		t.Errorf("took %s and said %q; want the early exit reported promptly", time.Since(start), out)
	}
}

func TestEngineStopOnlyStopsItsOwnServer(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	script := copyScripts(t, filepath.Join(checkout, "scripts"))
	if out, err := r.script(script, "start"); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}

	// Some other project's FastAPI app, started the common way.
	other := exec.Command("python3", "-c", "import time; time.sleep(30)", "uvicorn", "server:app")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer other.Process.Kill()

	if out, err := r.script(script, "stop", "--keep-autostart"); err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if r.healthy() {
		t.Error("stop left our server running")
	}
	if err := other.Process.Signal(syscallZero); err != nil {
		t.Error("stop killed an unrelated `uvicorn server:app` process")
	}
}

func TestEngineStopRespectsKeepAutostart(t *testing.T) {
	r := newEngineRun(t)
	script := copyScripts(t, filepath.Join(engineLayout(t), "scripts"))

	r.script(script, "stop", "--keep-autostart")
	if cfg, _ := os.ReadFile(r.config); !strings.Contains(string(cfg), `"engineAutoStart": true`) {
		t.Errorf("stop --keep-autostart changed the config: %s", cfg)
	}
	out, _ := r.script(script, "stop")
	if cfg, _ := os.ReadFile(r.config); !strings.Contains(string(cfg), `"engineAutoStart": false`) {
		t.Errorf("a plain stop did not disarm auto-start: %s", cfg)
	}
	// The disclosure is the point: a stop that prints only "Server stopped"
	// is how a caller finds out about engineAutoStart later, from hooks that
	// have quietly gone silent.
	if !strings.Contains(out, "Hook auto-start is now OFF") {
		t.Errorf("a plain stop did not say it turned auto-start off: %q", out)
	}
}

// start and the menu bar decide "running" by /health; status used to look
// only at the pid file, so it called a working server STOPPED.
func TestEngineStatusSeesAServerWithoutAPidFile(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	script := copyScripts(t, filepath.Join(checkout, "scripts"))
	if out, err := r.script(script, "start"); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	os.Remove(r.pid)

	out, _ := r.script(script, "status")
	if !strings.Contains(out, "RUNNING") {
		t.Errorf("status = %q with /health answering, want RUNNING", out)
	}
}

// stubPython stands in for the engine venv's python3: it records how it was
// run and prints a snapshot path, as `server.py fetch` does.
const stubPython = `#!/bin/sh
echo "$@" > "$STUB_OUT/python-argv"
pwd > "$STUB_OUT/python-cwd"
echo /fake/hf-cache/kokoro-snapshot
`

// `ava setup` downloads Kokoro through this, so it must run the engine's own
// loader, in the engine's own environment, from wherever the engine lives.
func TestEngineFetchRunsTheEnginesOwnDownload(t *testing.T) {
	r := newEngineRun(t)
	checkout := engineLayout(t)
	os.WriteFile(filepath.Join(checkout, "mlx-engine", ".venv", "bin", "python"), []byte(stubPython), 0755)
	script := copyScripts(t, filepath.Join(checkout, "scripts"))

	out, err := r.script(script, "fetch")
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, out)
	}
	if argv := strings.TrimSpace(r.saw("python-argv")); !strings.HasSuffix(argv, "server.py fetch") {
		t.Errorf("fetch ran python with %q, want it to end in %q", argv, "server.py fetch")
	}
	if cwd := r.saw("python-cwd"); !strings.Contains(cwd, filepath.Join(filepath.Base(checkout), "mlx-engine")) {
		t.Errorf("fetch ran in %q, want %s/mlx-engine", cwd, checkout)
	}
	if !strings.Contains(out, "/fake/hf-cache/kokoro-snapshot") {
		t.Errorf("fetch output %q does not say where the model is", out)
	}
	if r.healthy() {
		t.Error("fetch started the server; it should only download")
	}
}

func TestEngineFetchFailsFastWithNoEngine(t *testing.T) {
	r := newEngineRun(t)
	script := copyScripts(t, filepath.Join(t.TempDir(), "scripts"))

	out, err := r.script(script, "fetch")
	if err == nil {
		t.Fatalf("fetch with no engine exited 0:\n%s", out)
	}
	if !strings.Contains(out, "ava setup") {
		t.Errorf("fetch with no engine said %q; want it to point at `ava setup`", out)
	}
}
