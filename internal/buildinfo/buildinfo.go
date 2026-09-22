// Package buildinfo reports which build of ava is running.
//
// There was no way to ask before, which made bug reports unanswerable: a stale
// binary sitting in ~/.local/bin advertises whatever it was compiled from, and
// nobody — reporter or maintainer — can tell it apart from a current one. The
// MCP handshake carried a hand-maintained "0.1.0" that matched the tag only by
// coincidence and would have gone stale at the next one.
//
// Version comes from three places, most specific first:
//
//  1. -ldflags "-X .../internal/buildinfo.Version=..." — what `make build`
//     passes, from `git describe`, so a dev build names its exact commit.
//  2. the module version the Go toolchain records for `go install ...@latest`,
//     which gets no ldflags at all.
//  3. "dev", for `go run` and `go build` with neither.
package buildinfo

import "runtime/debug"

// Version is overwritten at link time. Do not rename without updating the
// -X flag in the Makefile, which fails silently on a path that no longer
// resolves — the build still succeeds and just reports "dev".
var Version = ""

// Get returns a human-readable build identifier, never empty.
func Get() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// "(devel)" is what the toolchain records for a local build, which is
		// no more informative than our own fallback.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
