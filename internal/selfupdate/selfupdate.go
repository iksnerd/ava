// Package selfupdate replaces the running ava binary with the latest release.
//
// The version comes from GitHub's latest-release API and the archive from that
// release, never from a mirror: a mirror's checksums.txt can lag a fresh
// release, and an updater that trusts it reports "up to date" on the day a
// release ships. The archive is checked against the release's checksums.txt
// before anything on disk changes, and the binary is replaced by renaming a
// file written beside it, so a failure at any step leaves the old ava running.
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultAPIURL       = "https://api.github.com/repos/iksnerd/ava/releases/latest"
	defaultReleasesBase = "https://github.com/iksnerd/ava/releases/download/"

	// maxDownload bounds one response. The archive carries the binary with the
	// embedded engine bundle, so it is tens of megabytes, not hundreds.
	maxDownload = 256 << 20
)

var (
	// ErrChecksum means the archive does not match the checksum its release
	// published. The running binary was not touched.
	ErrChecksum = errors.New("checksum mismatch")
	// ErrNotRelease means the running binary is not a tagged release (a dev
	// build or a build from a commit after a tag), so which release is newer
	// is not a question with an answer. --force replaces it anyway.
	ErrNotRelease = errors.New("this is not a release build")

	releaseTag = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)$`)
)

// Options says what to update and where to look. The zero value of every
// field but Current, Exe, GOOS and GOARCH means the default.
type Options struct {
	Current string // the running version, as buildinfo.Get reports it
	Exe     string // the binary to replace
	GOOS    string
	GOARCH  string
	Force   bool // reinstall the same version, downgrade, or replace a non-release build

	// Source is a base URL that serves checksums.txt and the archives under
	// their versioned names, to try a build before it is released. When set
	// the latest-release API is not asked.
	Source string

	APIURL       string // the latest-release endpoint; GitHub's by default
	ReleasesBase string // where releases are downloaded from; GitHub's by default
	Client       *http.Client
	Out          io.Writer
}

// Result says what Run did.
type Result struct {
	From    string // the version that was running, without a leading v
	To      string // the latest version found
	Updated bool
}

// Run updates opts.Exe to the latest release when that is newer than
// opts.Current. It returns an error and leaves the binary alone for anything
// it cannot verify.
func Run(ctx context.Context, opts Options) (Result, error) {
	opts = opts.withDefaults()
	res := Result{From: strings.TrimPrefix(opts.Current, "v")}

	current := releaseTag.FindStringSubmatch(opts.Current)
	if current == nil && !opts.Force {
		return res, fmt.Errorf("%w (%s); run `ava update --force` to replace it with the latest release", ErrNotRelease, opts.Current)
	}

	base := opts.Source
	latest := ""
	if base == "" {
		var err error
		if latest, err = latestVersion(ctx, opts); err != nil {
			return res, err
		}
		base = opts.ReleasesBase + "v" + latest
	}
	base = strings.TrimRight(base, "/")

	sums, err := fetch(ctx, opts.Client, base+"/checksums.txt")
	if err != nil {
		return res, fmt.Errorf("fetch checksums: %w", err)
	}
	shaByFile := parseChecksums(string(sums))

	name, version := archiveFor(shaByFile, opts.GOOS, opts.GOARCH)
	if name == "" {
		return res, fmt.Errorf("no ava release for %s/%s in %s/checksums.txt", opts.GOOS, opts.GOARCH, base)
	}
	if latest != "" && version != latest {
		return res, fmt.Errorf("the latest release is %s but its checksums name %s", latest, version)
	}
	res.To = version

	if current != nil && !opts.Force && compareVersions(version, current[1]) <= 0 {
		fmt.Fprintf(opts.Out, "ava %s is up to date (latest release %s)\n", current[1], version)
		return res, nil
	}

	fmt.Fprintf(opts.Out, "Updating ava %s -> %s\n", res.From, version)
	archive, err := fetch(ctx, opts.Client, base+"/"+name)
	if err != nil {
		return res, fmt.Errorf("download %s: %w", name, err)
	}
	got := sha256.Sum256(archive)
	if want := shaByFile[name]; hex.EncodeToString(got[:]) != want {
		return res, fmt.Errorf("%w for %s: got %x, want %s; the installed ava was not changed", ErrChecksum, name, got, want)
	}
	binary, err := extractAva(archive)
	if err != nil {
		return res, fmt.Errorf("%s: %w", name, err)
	}
	if err := replaceBinary(opts.Exe, binary); err != nil {
		return res, fmt.Errorf("replace %s: %w", opts.Exe, err)
	}
	res.Updated = true
	fmt.Fprintf(opts.Out, "Updated %s to %s\n", opts.Exe, version)
	return res, nil
}

func (o Options) withDefaults() Options {
	if o.APIURL == "" {
		o.APIURL = defaultAPIURL
	}
	if o.ReleasesBase == "" {
		o.ReleasesBase = defaultReleasesBase
	}
	if !strings.HasSuffix(o.ReleasesBase, "/") {
		o.ReleasesBase += "/"
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	return o
}

func latestVersion(ctx context.Context, opts Options) (string, error) {
	body, err := fetch(ctx, opts.Client, opts.APIURL)
	if err != nil {
		return "", fmt.Errorf("look up the latest release: %w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("decode the latest release: %w", err)
	}
	m := releaseTag.FindStringSubmatch(strings.TrimSpace(release.TagName))
	if m == nil {
		return "", fmt.Errorf("the latest release has tag %q, which is not a version", release.TagName)
	}
	return m[1], nil
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ava-update")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDownload {
		return nil, fmt.Errorf("GET %s: more than %d MiB", url, maxDownload>>20)
	}
	return body, nil
}

// parseChecksums reads GoReleaser's "<sha256>  <file>" lines.
func parseChecksums(s string) map[string]string {
	out := map[string]string{}
	for line := range strings.SplitSeq(s, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			out[f[1]] = f[0]
		}
	}
	return out
}

var archiveName = regexp.MustCompile(`^ava_(\d+\.\d+\.\d+)_([a-z0-9]+)_([a-z0-9]+)\.tar\.gz$`)

// archiveFor finds this platform's archive among the checksummed files and
// the version it carries, or "" when the release has none.
func archiveFor(shaByFile map[string]string, goos, goarch string) (name, version string) {
	for file := range shaByFile {
		if m := archiveName.FindStringSubmatch(file); m != nil && m[2] == goos && m[3] == goarch {
			return file, m[1]
		}
	}
	return "", ""
}

// compareVersions orders two X.Y.Z versions numerically.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// extractAva returns the contents of the file named ava at the top of the
// archive. Other entries are never written anywhere.
func extractAva(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("the archive has no ava binary")
		}
		if err != nil {
			return nil, fmt.Errorf("untar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || path.Clean(hdr.Name) != "ava" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxDownload+1))
		if err != nil {
			return nil, fmt.Errorf("read ava from the archive: %w", err)
		}
		if len(data) > maxDownload {
			return nil, fmt.Errorf("the ava binary in the archive is over %d MiB", maxDownload>>20)
		}
		return data, nil
	}
}

// replaceBinary swaps dst for data by writing a file beside it and renaming it
// over dst. On macOS a rename over a running binary is fine: the process keeps
// the old inode until it exits.
func replaceBinary(dst string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".new-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once the rename has happened
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, dst)
}
