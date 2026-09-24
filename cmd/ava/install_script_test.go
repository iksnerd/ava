package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease writes the two assets a GoReleaser release publishes: the
// archive under its real name and a checksums.txt that names it.
func fakeRelease(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	name := "ava_" + version + "_darwin_arm64.tar.gz"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho ava " + version + "\n")
	tw.WriteHeader(&tar.Header{Name: "ava", Mode: 0755, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	gz.Close()
	f.Close()

	data, _ := os.ReadFile(filepath.Join(dir, name))
	sum := sha256.Sum256(data)
	os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(hex.EncodeToString(sum[:])+"  "+name+"\n"), 0644)
	return dir
}

// The README's one-liner runs install.sh through curl on a machine that
// may have no gh at all. That path saved the archive as ava.tar.gz, while
// checksums.txt names the real file, so the check verified nothing and the
// install refused: the first command a new user runs failed.
func TestInstallScriptWorksWithCurlAlone(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("install.sh only installs on Apple Silicon macOS")
	}
	release := fakeRelease(t, "9.9.9")

	// A curl that serves the release from disk: `curl -fsSL -o <out> <url>`.
	stub := t.TempDir()
	script := `#!/bin/bash
out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift 2;; -*) shift;; *) url="$1"; shift;; esac; done
src="` + release + `/$(basename "$url")"
[ -f "$src" ] || { echo "curl: (22) 404 $url" >&2; exit 22; }
cp "$src" "$out"
`
	os.WriteFile(filepath.Join(stub, "curl"), []byte(script), 0755)

	bin := t.TempDir()
	cmd := exec.Command("bash", filepath.Join("..", "..", "scripts", "install.sh"), "v9.9.9")
	// System dirs only: no Homebrew, so no gh, like a fresh Mac.
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + stub + ":/usr/bin:/bin:/usr/sbin:/sbin", "AVA_BIN=" + bin}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh with curl alone failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(bin, "ava")); err != nil {
		t.Errorf("ava was not installed:\n%s", out)
	}
	if !strings.Contains(string(out), "Checksum verified") {
		t.Errorf("install did not verify the checksum:\n%s", out)
	}
}
