package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iksnerd/ava/internal/testutil"
)

// execRoot runs the whole command tree, stdout and stderr kept apart.
func execRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd()
	var o, e bytes.Buffer
	root.SetOut(&o)
	root.SetErr(&e)
	root.SetArgs(args)
	err = root.Execute()
	return o.String(), e.String(), err
}

// healthServer answers /health with body, standing in for an engine.
func healthServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readySetup(t *testing.T) *setupRig {
	skipOffAppleSilicon(t)
	r := newSetupRig(t)
	r.addTools(t)
	r.addModel(t)
	r.addEngine(t)
	return r
}

// A stale engine is running, so every speak fails with ErrStaleEngine, yet
// setup --check used to say "Ava is set up".
func TestSetupCheckReportsAStaleRunningEngine(t *testing.T) {
	r := readySetup(t)
	setupEngineURL = healthServer(t, `{"status":"ok"}`).URL
	t.Cleanup(func() { setupEngineURL = "" })

	out, err := r.check(t)
	if err == nil {
		t.Fatalf("setup --check passed with a stale engine running:\n%s", out)
	}
	if !strings.Contains(out, "❌ The running mlx-engine is older than this ava") ||
		!strings.Contains(out, "ava engine stop && ava engine start") {
		t.Errorf("output does not name the stale engine and the fix:\n%s", out)
	}
	if strings.Contains(out, "Missing") || strings.Contains(out, "set up") {
		t.Errorf("a stale engine is not a missing install:\n%s", out)
	}
}

func TestSetupCheckIgnoresAnEngineThatIsDown(t *testing.T) {
	r := readySetup(t)
	srv := healthServer(t, "")
	setupEngineURL = srv.URL
	t.Cleanup(func() { setupEngineURL = "" })
	srv.Close()

	if out, err := r.check(t); err != nil {
		t.Fatalf("a stopped engine failed setup --check: %v\n%s", err, out)
	}
}

func TestSetupCheckJSON(t *testing.T) {
	r := newSetupRig(t)
	setupEngineURL = "http://127.0.0.1:1"
	t.Cleanup(func() { setupEngineURL = "" })

	out, err := r.check(t, "--skip-engine", "--json")
	if err == nil {
		t.Fatal("incomplete setup must still exit non-zero with --json")
	}
	var got struct {
		OK      bool     `json:"ok"`
		Missing []string `json:"missing"`
		Stale   bool     `json:"stale_engine"`
	}
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("stdout is not only JSON: %v\n%q", jerr, out)
	}
	if got.OK || len(got.Missing) == 0 || got.Stale {
		t.Errorf("got %+v", got)
	}

	r.addTools(t)
	r.addModel(t)
	out, err = r.check(t, "--skip-engine", "--json")
	if err != nil || !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"missing":[]`) {
		t.Errorf("complete setup: err=%v out=%q", err, out)
	}
}

func TestSetupHelpSizesAddUpToTheReadme(t *testing.T) {
	out, _, _ := execRoot(t, "setup", "--help")
	if strings.Contains(out, "~1.5 GB") {
		t.Errorf("help still says ~1.5 GB:\n%s", out)
	}
	if !strings.Contains(out, "1.7 GB") {
		t.Errorf("help does not give the 1.7 GB total:\n%s", out)
	}
}

func TestUnknownEngineSubcommandIsAnError(t *testing.T) {
	for _, arg := range []string{"statu", "bogus"} {
		_, _, err := execRoot(t, "engine", arg)
		if err == nil || !strings.Contains(err.Error(), `unknown command "`+arg+`" for "ava engine"`) {
			t.Errorf("engine %s: err = %v", arg, err)
		}
	}
	if _, _, err := execRoot(t, "engine"); err != nil {
		t.Errorf("bare engine still shows help: %v", err)
	}
}

func TestUnknownRootCommandSuggests(t *testing.T) {
	_, _, err := execRoot(t, "vioces")
	if err == nil || !strings.Contains(err.Error(), `unknown command "vioces" for "ava"`) ||
		!strings.Contains(err.Error(), "voices") {
		t.Errorf("err = %v, want an unknown command error suggesting voices", err)
	}
	_, _, err = execRoot(t, "engine", "statu")
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("engine statu err = %v, want a suggestion of status", err)
	}
}

// statusScript is a control script that answers `status` the way the real one does.
func statusScript(t *testing.T, line string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ctl.sh")
	testutil.WriteExec(t, p, "#!/bin/sh\necho '"+line+"'\n")
	return p
}

func TestEngineStatusCheck(t *testing.T) {
	stopped := statusScript(t, "🔴 mlx-engine server is STOPPED")
	running := statusScript(t, "🟢 mlx-engine server is RUNNING (PID: 4242)")

	out, _, err := execRoot(t, "engine", "status", "--script", stopped)
	if err != nil || !strings.Contains(out, "STOPPED") {
		t.Errorf("default status must stay exit 0 and verbose: err=%v out=%q", err, out)
	}
	out, errOut, err := execRoot(t, "engine", "status", "--check", "--script", stopped)
	if err == nil || out != "" || errOut != "" {
		t.Errorf("--check stopped: err=%v out=%q err=%q, want quiet non-zero", err, out, errOut)
	}
	if !isSilentExit(err) {
		t.Errorf("--check must fail without Execute printing a message, got %T", err)
	}
	out, _, err = execRoot(t, "engine", "status", "--check", "--script", running)
	if err != nil || out != "" {
		t.Errorf("--check running: err=%v out=%q", err, out)
	}
}

func TestEngineStatusJSON(t *testing.T) {
	out, _, err := execRoot(t, "engine", "status", "--json", "--script",
		statusScript(t, "🟢 mlx-engine server is RUNNING (PID: 4242)"))
	if err != nil || strings.TrimSpace(out) != `{"running":true,"pid":4242}` {
		t.Errorf("running: err=%v out=%q", err, out)
	}
	out, _, err = execRoot(t, "engine", "status", "--json", "--script",
		statusScript(t, "🟢 mlx-engine server is RUNNING on http://x (not started by this script, so no PID)"))
	if err != nil || strings.TrimSpace(out) != `{"running":true,"pid":null}` {
		t.Errorf("running without pid: err=%v out=%q", err, out)
	}
	out, _, err = execRoot(t, "engine", "status", "--json", "--script",
		statusScript(t, "🔴 mlx-engine server is STOPPED"))
	if err != nil || strings.TrimSpace(out) != `{"running":false,"pid":null}` {
		t.Errorf("stopped: err=%v out=%q (exit stays 0 without --check)", err, out)
	}
}

func TestVoicesJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, errOut, err := execRoot(t, "voices", "--json")
	if err != nil || errOut != "" {
		t.Fatalf("err=%v stderr=%q", err, errOut)
	}
	var got struct {
		Current string   `json:"current"`
		Voices  []string `json:"voices"`
	}
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("stdout is not only JSON: %v\n%q", jerr, out)
	}
	if got.Current == "" || len(got.Voices) < 2 {
		t.Errorf("got %+v", got)
	}
}

func TestVersionJSON(t *testing.T) {
	out, _, err := execRoot(t, "--version", "--json")
	var got struct {
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || got.Version == "" {
		t.Errorf("err=%v out=%q", err, out)
	}
	plain, _, _ := execRoot(t, "--version")
	if !strings.Contains(plain, "version") || strings.Contains(plain, "{") {
		t.Errorf("plain --version changed: %q", plain)
	}
}

func TestSpeakRejectsABadServerURLNamingIt(t *testing.T) {
	for _, bad := range []string{"notaurl", "ftp://127.0.0.1:1", "http://"} {
		_, _, err := execRoot(t, "speak", "--server-url", bad, "hi")
		if err == nil || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "--server-url") {
			t.Errorf("--server-url %q: err = %v, want a message naming it", bad, err)
		}
	}
}
