package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// `make install-raycast` writes a launcher into the user's home directory,
// where no check in this repo will ever read it again. That is how it kept
// emitting `--engine=whisper` for a release after the flag was deleted: the
// flag lives inside a Makefile `@echo` line, which reads like prose to a grep
// aimed at call sites, and the generated file itself is outside the repo.
//
// This reads the flags back out of the Makefile's generator and asserts the
// root command still has them. It fails when the two DISAGREE, which is the
// only version that catches a flag removal — asserting the Makefile contains
// some expected string would need updating by the same person who forgot.
func TestRaycastGeneratorUsesOnlyRealFlags(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}

	// The generator's payload lines look like:
	//   @echo "exec $(abspath $(BINARY_PATH)) --some-flag" >> $(RAYCAST_DIR)/...
	execLine := regexp.MustCompile(`@echo "exec \$\(abspath \$\(BINARY_PATH\)\)([^"]*)"`)
	matches := execLine.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		t.Fatal("found no `@echo \"exec $(abspath $(BINARY_PATH))...\"` line in the Makefile — " +
			"if install-raycast was rewritten, update this pattern rather than deleting the test")
	}

	root := newRootCmd()
	flagToken := regexp.MustCompile(`--([A-Za-z0-9-]+)`)

	for _, m := range matches {
		args := string(m[1])
		for _, f := range flagToken.FindAllStringSubmatch(args, -1) {
			name := f[1]
			if root.Flags().Lookup(name) == nil && root.PersistentFlags().Lookup(name) == nil {
				t.Errorf("the Raycast launcher the Makefile generates passes --%s, "+
					"which ava's root command does not define — running it "+
					"exits on `unknown flag` (generated line: exec ...%s)",
					name, strings.TrimSpace(args))
			}
		}
	}
}
