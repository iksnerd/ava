package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	oldBinary = "old ava binary"
	newBinary = "new ava binary"
)

// release is what the fake GitHub and its download host serve.
type release struct {
	tag      string // "v0.10.0"
	nameVer  string // the version the archive names carry, when it is not the tag's
	binary   string // contents of the ava file in the archive
	sumFor   string // contents the checksum is computed over; "" means the real archive
	apiDown  bool
	archive  []byte
	hits     []string
	notFound map[string]bool
}

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// serve starts the fake host and returns Options pointed at it, with Exe a
// real file holding oldBinary in a temp dir.
func serve(t *testing.T, r *release) (Options, string) {
	t.Helper()
	ver := strings.TrimPrefix(r.tag, "v")
	if r.nameVer != "" {
		ver = r.nameVer
	}
	name := fmt.Sprintf("ava_%s_darwin_arm64.tar.gz", ver)
	r.archive = tarball(t, map[string]string{"ava": r.binary, "README.md": "readme", "LICENSE": "license"})
	over := r.archive
	if r.sumFor != "" {
		over = []byte(r.sumFor)
	}
	checksums := fmt.Sprintf("%s  %s\n%s  ava_%s_linux_amd64.tar.gz\n", sum(over), name, strings.Repeat("0", 64), ver)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, req *http.Request) {
		r.hits = append(r.hits, req.URL.Path)
		if r.apiDown {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"tag_name": %q}`, r.tag)
	})
	mux.HandleFunc("/releases/", func(w http.ResponseWriter, req *http.Request) {
		r.hits = append(r.hits, req.URL.Path)
		if r.notFound[filepath.Base(req.URL.Path)] {
			http.NotFound(w, req)
			return
		}
		switch req.URL.Path {
		case "/releases/" + r.tag + "/checksums.txt":
			io.WriteString(w, checksums)
		case "/releases/" + r.tag + "/" + name:
			w.Write(r.archive)
		default:
			http.NotFound(w, req)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	exe := filepath.Join(t.TempDir(), "ava")
	if err := os.WriteFile(exe, []byte(oldBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	return Options{
		Current:      "v0.9.0",
		Exe:          exe,
		GOOS:         "darwin",
		GOARCH:       "arm64",
		APIURL:       srv.URL + "/api/latest",
		ReleasesBase: srv.URL + "/releases/",
		Client:       srv.Client(),
		Out:          io.Discard,
	}, srv.URL
}

func contents(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func noLeftovers(t *testing.T, exe string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(exe) {
			t.Errorf("left %s beside the binary", e.Name())
		}
	}
}

func TestUpdateReplacesTheBinaryWithTheLatestRelease(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary})

	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || res.From != "0.9.0" || res.To != "0.10.0" {
		t.Errorf("result = %+v, want updated 0.9.0 -> 0.10.0", res)
	}
	if got := contents(t, opts.Exe); got != newBinary {
		t.Errorf("binary = %q, want the new one", got)
	}
	info, err := os.Stat(opts.Exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	noLeftovers(t, opts.Exe)
}

func TestUpdateWhenAlreadyCurrentLeavesTheBinaryAndDownloadsNoArchive(t *testing.T) {
	r := &release{tag: "v0.9.0", binary: newBinary}
	opts, _ := serve(t, r)

	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated {
		t.Error("reported an update when already on the latest")
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed to %q", got)
	}
	for _, h := range r.hits {
		if strings.HasSuffix(h, ".tar.gz") {
			t.Errorf("downloaded %s when already current", h)
		}
	}
}

func TestUpdateNeverDowngradesWithoutForce(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.8.1", binary: newBinary})

	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated || contents(t, opts.Exe) != oldBinary {
		t.Error("installed an older release over a newer binary")
	}
}

func TestForceReinstallsTheSameVersion(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.9.0", binary: newBinary})
	opts.Force = true

	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || contents(t, opts.Exe) != newBinary {
		t.Errorf("--force did not reinstall: %+v", res)
	}
}

// The case the updater exists to get right: a download that is not the one the
// checksums name must not touch the running binary.
func TestChecksumMismatchLeavesTheOldBinaryUntouched(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary, sumFor: "some other archive"})

	_, err := Run(context.Background(), opts)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary = %q after a checksum mismatch", got)
	}
	noLeftovers(t, opts.Exe)
}

func TestUpdateRefusesADevBuildWithoutForce(t *testing.T) {
	for _, current := range []string{"dev", "v0.9.0-3-gabc1234", "v0.9.0-dirty"} {
		t.Run(current, func(t *testing.T) {
			opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary})
			opts.Current = current

			_, err := Run(context.Background(), opts)
			if !errors.Is(err, ErrNotRelease) {
				t.Fatalf("err = %v, want ErrNotRelease", err)
			}
			if got := contents(t, opts.Exe); got != oldBinary {
				t.Errorf("replaced a dev build: %q", got)
			}
		})
	}
}

func TestForceReplacesADevBuild(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary})
	opts.Current = "dev"
	opts.Force = true

	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, opts.Exe); got != newBinary {
		t.Errorf("binary = %q, want the new one", got)
	}
}

func TestUpdateFailsWhenThereIsNoArchiveForThisPlatform(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary})
	opts.GOOS, opts.GOARCH = "windows", "amd64"

	_, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "windows/amd64") {
		t.Fatalf("err = %v, want one naming windows/amd64", err)
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed: %q", got)
	}
}

func TestUpdateFailsWhenTheArchiveHasNoAvaBinary(t *testing.T) {
	r := &release{tag: "v0.10.0", binary: newBinary}
	opts, _ := serve(t, r)
	// Serve an archive without "ava" in it, with a checksum that matches.
	r.archive = tarball(t, map[string]string{"README.md": "readme"})
	r.sumFor = ""
	opts2, _ := serveArchive(t, r)
	opts.APIURL, opts.ReleasesBase, opts.Client = opts2.APIURL, opts2.ReleasesBase, opts2.Client

	_, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "no ava binary") {
		t.Fatalf("err = %v, want one saying the archive has no ava binary", err)
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed: %q", got)
	}
}

// serveArchive serves r.archive as it is, with a checksum over those bytes.
func serveArchive(t *testing.T, r *release) (Options, string) {
	t.Helper()
	ver := strings.TrimPrefix(r.tag, "v")
	name := fmt.Sprintf("ava_%s_darwin_arm64.tar.gz", ver)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name": %q}`, r.tag)
	})
	mux.HandleFunc("/releases/"+r.tag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sum(r.archive), name)
	})
	mux.HandleFunc("/releases/"+r.tag+"/"+name, func(w http.ResponseWriter, _ *http.Request) { w.Write(r.archive) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return Options{APIURL: srv.URL + "/api/latest", ReleasesBase: srv.URL + "/releases/", Client: srv.Client()}, srv.URL
}

func TestUpdateReportsAFailedLookupAndChangesNothing(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary, apiDown: true})

	_, err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("no error when the release lookup fails")
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed: %q", got)
	}
}

// --source points at a host that serves checksums.txt and the archives itself,
// for trying a build before it is released. The API is not consulted.
func TestSourceOverridesTheReleaseLookup(t *testing.T) {
	r := &release{tag: "v0.10.0", binary: newBinary}
	opts, base := serve(t, r)
	opts.APIURL = "http://127.0.0.1:1/never"
	opts.Source = base + "/releases/v0.10.0"

	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.To != "0.10.0" || contents(t, opts.Exe) != newBinary {
		t.Errorf("result %+v, binary %q", res, contents(t, opts.Exe))
	}
}

func TestACancelledContextStopsTheUpdateBeforeTheBinaryChanges(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", binary: newBinary})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want one wrapping context.Canceled", err)
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed: %q", got)
	}
}

func TestParseChecksums(t *testing.T) {
	in := "aaa  ava_0.10.0_darwin_arm64.tar.gz\n\nmalformed\nthree fields here\nbbb  other.txt\n"
	got := parseChecksums(in)
	if got["ava_0.10.0_darwin_arm64.tar.gz"] != "aaa" || got["other.txt"] != "bbb" || len(got) != 2 {
		t.Errorf("parseChecksums = %v", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.9.0", "0.10.0", -1}, // numeric, not lexical
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"0.9.1", "0.9.0", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// The tag says one version and the files it holds say another: something is
// wrong with that release, and installing from it would be a guess.
func TestUpdateRefusesAReleaseWhoseFilesNameAnotherVersion(t *testing.T) {
	opts, _ := serve(t, &release{tag: "v0.10.0", nameVer: "0.9.5", binary: newBinary})

	_, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "0.9.5") {
		t.Fatalf("err = %v, want one naming the mismatched version", err)
	}
	if got := contents(t, opts.Exe); got != oldBinary {
		t.Errorf("binary changed: %q", got)
	}
}

func TestArchiveForPicksTheArchiveOfThisOSAndArch(t *testing.T) {
	sums := map[string]string{
		"ava_0.10.0_darwin_amd64.tar.gz": "a",
		"ava_0.10.0_darwin_arm64.tar.gz": "b",
		"ava_0.10.0_linux_arm64.tar.gz":  "c",
		"checksums.txt":                  "d",
	}
	for _, c := range []struct{ os, arch, want string }{
		{"darwin", "arm64", "ava_0.10.0_darwin_arm64.tar.gz"},
		{"darwin", "amd64", "ava_0.10.0_darwin_amd64.tar.gz"},
		{"linux", "arm64", "ava_0.10.0_linux_arm64.tar.gz"},
		{"linux", "amd64", ""},
	} {
		if got, _ := archiveFor(sums, c.os, c.arch); got != c.want {
			t.Errorf("archiveFor(%s/%s) = %q, want %q", c.os, c.arch, got, c.want)
		}
	}
}
