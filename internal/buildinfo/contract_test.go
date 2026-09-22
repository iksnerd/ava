package buildinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Version is set through an -X linker flag, which fails *silently* when its
// path stops resolving: the build succeeds and --version quietly reports the
// fallback. The package comment has warned about that since the flag existed,
// and there is now a second place spelling the same path — .goreleaser.yaml —
// so a rename would have to be caught in both.
//
// This fails when the two DISAGREE, and separately when either names a symbol
// this package does not export. A test that only checked the Makefile would
// pass while every released binary reported "dev".

var xFlag = regexp.MustCompile(`-X\s+([A-Za-z0-9_./-]+)\.([A-Za-z0-9_]+)=`)

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestVersionStampingAgreesAcrossBuildSystems(t *testing.T) {
	make := xFlag.FindStringSubmatch(readRepoFile(t, "Makefile"))
	if make == nil {
		t.Fatal("the Makefile no longer passes an -X flag; if version stamping moved, " +
			"update this test rather than deleting it")
	}
	rel := xFlag.FindStringSubmatch(readRepoFile(t, ".goreleaser.yaml"))
	if rel == nil {
		t.Fatal(".goreleaser.yaml no longer passes an -X flag, so released binaries " +
			"would report the fallback version instead of the tag")
	}

	if make[1] != rel[1] || make[2] != rel[2] {
		t.Errorf("the Makefile stamps %s.%s but .goreleaser.yaml stamps %s.%s — "+
			"one of them is writing to a symbol that does not exist, and an -X flag "+
			"that misses fails silently",
			make[1], make[2], rel[1], rel[2])
	}

	// Both must name a variable this package actually has. A rename here is the
	// original hazard, and neither build system would complain about it.
	const pkg = "github.com/iksnerd/local-whisper/internal/buildinfo"
	if make[1] != pkg {
		t.Errorf("the -X flag targets %q, but this package is %q", make[1], pkg)
	}
	if make[2] != "Version" {
		t.Errorf("the -X flag sets %q; this package exports Version", make[2])
	}
}

// GoReleaser must stamp the same shape `make build` does, or a released
// binary and a local one report versions that cannot be compared in a bug
// report — which is the problem buildinfo exists to solve.
func TestGoreleaserStampsTheSameVersionShapeAsMake(t *testing.T) {
	body := readRepoFile(t, ".goreleaser.yaml")
	// .Summary is GoReleaser's `git describe --tags --always --dirty`, which is
	// exactly what the Makefile's VERSION runs. .Version would drop the leading
	// "v" and the commit suffix.
	if !regexp.MustCompile(`buildinfo\.Version=\{\{\s*\.Summary\s*\}\}`).MatchString(body) {
		t.Error(".goreleaser.yaml does not stamp {{ .Summary }}. Anything else " +
			"(notably .Version, which strips the leading v and the commit) makes a " +
			"released binary's --version incomparable with a local build's.")
	}
}
