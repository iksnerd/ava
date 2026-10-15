package archtest

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// graph is a hand-built import graph: the checker is tested on these so a
// failure here means the checker is wrong, not that the module moved.
func graph() Graph {
	return Graph{
		"m/pkg/a":      {"m/pkg/b", "std"},
		"m/pkg/b":      {"m/internal/x"},
		"m/internal/x": {"ext/cobra"},
		"ext/cobra":    {"std"},
		"std":          nil,
		"m/cmd/app":    {"m/pkg/a", "ext/cobra"},
	}
}

func TestCheckPassesCleanGraph(t *testing.T) {
	rules := []Rule{{Name: "no-mcp", From: []string{"m/pkg/..."}, Forbid: []string{"ext/mcp/..."}}}
	if got := Check(graph(), rules); len(got) != 0 {
		t.Fatalf("clean graph flagged: %v", got)
	}
}

func TestCheckFlagsDirectImport(t *testing.T) {
	rules := []Rule{{Name: "no-x", From: []string{"m/pkg/b"}, Forbid: []string{"m/internal/x"}, Direct: true}}
	got := Check(graph(), rules)
	want := []string{`rule "no-x": m/pkg/b imports m/internal/x directly`}
	if len(got) != 1 || !strings.HasPrefix(got[0], want[0]) {
		t.Fatalf("got %q, want prefix %q", got, want)
	}
}

func TestCheckDirectIgnoresTransitive(t *testing.T) {
	rules := []Rule{{Name: "no-cobra", From: []string{"m/pkg/a"}, Forbid: []string{"ext/cobra"}, Direct: true}}
	if got := Check(graph(), rules); len(got) != 0 {
		t.Fatalf("direct rule flagged a transitive import: %v", got)
	}
}

func TestCheckFlagsTransitiveWithChain(t *testing.T) {
	rules := []Rule{{Name: "core-free-of-cobra", From: []string{"m/pkg/..."}, Forbid: []string{"ext/cobra"}}}
	got := Check(graph(), rules)
	if len(got) != 2 {
		t.Fatalf("want a violation for pkg/a and pkg/b, got %q", got)
	}
	if !strings.Contains(got[0], "m/pkg/a -> m/pkg/b -> m/internal/x -> ext/cobra") {
		t.Errorf("chain missing from %q", got[0])
	}
	if !strings.Contains(got[1], "m/pkg/b -> m/internal/x -> ext/cobra") ||
		!strings.Contains(got[1], `rule "core-free-of-cobra"`) {
		t.Errorf("bad message %q", got[1])
	}
}

func TestCheckExceptSkipsPackages(t *testing.T) {
	rules := []Rule{{Name: "r", From: []string{"m/..."}, Except: []string{"m/cmd/..."}, Forbid: []string{"m/cmd/..."}}}
	if got := Check(graph(), rules); len(got) != 0 {
		t.Fatalf("Except not honoured: %v", got)
	}
	rules[0].Except = nil
	if got := Check(graph(), rules); len(got) != 0 {
		// cmd/app imports nothing under cmd: still clean, but now cmd/app is in scope.
		t.Fatalf("unexpected: %v", got)
	}
}

func TestCheckRuleMatchingNothingFails(t *testing.T) {
	rules := []Rule{{Name: "typo", From: []string{"m/pgk/..."}, Forbid: []string{"ext/cobra"}}}
	got := Check(graph(), rules)
	if len(got) != 1 || !strings.Contains(got[0], "matches no package") {
		t.Fatalf("a rule that covers nothing must fail, got %q", got)
	}
}

func TestCheckDeterministic(t *testing.T) {
	rules := []Rule{
		{Name: "z", From: []string{"m/pkg/..."}, Forbid: []string{"ext/cobra"}},
		{Name: "a", From: []string{"m/pkg/..."}, Forbid: []string{"std"}},
	}
	first := Check(graph(), rules)
	for i := 0; i < 20; i++ {
		if got := Check(graph(), rules); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs:\n%q\n%q", i, got, first)
		}
	}
}

func TestCheckShortestChain(t *testing.T) {
	g := Graph{"a": {"b", "x"}, "b": {"c"}, "c": {"bad"}, "x": {"bad"}, "bad": nil}
	got := Check(g, []Rule{{Name: "r", From: []string{"a"}, Forbid: []string{"bad"}}})
	if len(got) != 1 || !strings.Contains(got[0], "a -> x -> bad") {
		t.Fatalf("want the shortest chain, got %q", got)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{"m/pkg", "m/pkg", true},
		{"m/pkg", "m/pkg/sub", false},
		{"m/pkg/...", "m/pkg", true},
		{"m/pkg/...", "m/pkg/sub", true},
		{"m/pkg/...", "m/pkgx", false},
	}
	for _, c := range cases {
		if got := match(c.pat, c.path); got != c.want {
			t.Errorf("match(%q,%q)=%v", c.pat, c.path, got)
		}
	}
}

// Except has to change the result. The package it excepts here makes the
// forbidden import, so the rule only passes because of Except; remove it and the
// same package is flagged beside the one that is always in scope.
func TestCheckExceptChangesTheResult(t *testing.T) {
	g := Graph{"m/core": {"m/cmd/x"}, "m/cmd/y": {"m/cmd/x"}, "m/cmd/x": nil}
	rule := Rule{Name: "only-cmd-imports-cmd", From: []string{"m/..."}, Except: []string{"m/cmd/..."}, Forbid: []string{"m/cmd/..."}, Direct: true}

	got := Check(g, []Rule{rule})
	if len(got) != 1 || !strings.Contains(got[0], "m/core imports m/cmd/x") {
		t.Fatalf("with Except only m/core should be flagged, got %q", got)
	}
	rule.Except = nil
	got = Check(g, []Rule{rule})
	if len(got) != 2 || !strings.Contains(got[0], "m/cmd/y imports m/cmd/x") {
		t.Fatalf("without Except m/cmd/y is in scope too, got %q", got)
	}
}

// The output is sorted, not merely stable: a rule listed second whose name sorts
// first comes first. TestCheckDeterministic only reruns the same input.
func TestCheckOutputIsSortedAcrossRules(t *testing.T) {
	rules := []Rule{
		{Name: "z-rule", From: []string{"m/pkg/b"}, Forbid: []string{"ext/cobra"}},
		{Name: "a-rule", From: []string{"m/pkg/b"}, Forbid: []string{"m/internal/x"}},
	}
	got := Check(graph(), rules)
	if len(got) != 2 || !sort.StringsAreSorted(got) || !strings.HasPrefix(got[0], `rule "a-rule"`) {
		t.Fatalf("want a-rule before z-rule in sorted order, got %q", got)
	}
}

// Two chains of the same length: the one reported must not depend on the order
// the importer's imports were listed in (go list's order is not part of the
// contract), so the same tree gives the same message on every machine.
func TestCheckChainDoesNotDependOnImportOrder(t *testing.T) {
	forward := Graph{"a": {"b", "c"}, "b": {"bad"}, "c": {"bad"}, "bad": nil}
	reverse := Graph{"a": {"c", "b"}, "b": {"bad"}, "c": {"bad"}, "bad": nil}
	rules := []Rule{{Name: "r", From: []string{"a"}, Forbid: []string{"bad"}}}

	f, r := Check(forward, rules), Check(reverse, rules)
	if !reflect.DeepEqual(f, r) {
		t.Fatalf("import order changed the report:\n%q\n%q", f, r)
	}
	if len(f) != 1 || !strings.Contains(f[0], "a -> b -> bad") {
		t.Fatalf("want the alphabetically first of equal chains, got %q", f)
	}
}
