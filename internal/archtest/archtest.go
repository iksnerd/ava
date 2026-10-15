// Package archtest turns the dependency direction of ava's packages into
// executable rules. The layers are engines (out of process) < core library
// (pkg/ and the internal packages that will become public) < adapters (cmd/).
// Core must not depend on adapters, and a library consumer must not pay for
// the CLI framework. The rules live in rules.go; Check is the engine.
package archtest

import (
	"fmt"
	"sort"
	"strings"
)

// Graph maps an import path to its direct imports.
type Graph map[string][]string

// Rule forbids packages matching From (minus Except) from importing anything
// matching Forbid. A pattern is an exact import path, or "prefix/..." for the
// prefix itself and everything below it. Direct limits the rule to the
// package's own import list; otherwise the whole dependency closure counts.
type Rule struct {
	Name   string
	Why    string // what the rule protects, and what to move when it fails
	From   []string
	Except []string
	Forbid []string
	Direct bool
}

// Check returns one sorted message per violation. A rule whose From matches no
// package is itself a violation: a typo or a moved package would otherwise
// leave the rule passing over nothing.
func Check(g Graph, rules []Rule) []string {
	var out []string
	for _, r := range rules {
		covered := 0
		for _, pkg := range sortedKeys(g) {
			if !matchAny(r.From, pkg) || matchAny(r.Except, pkg) {
				continue
			}
			covered++
			if msg := violation(g, r, pkg); msg != "" {
				out = append(out, msg)
			}
		}
		if covered == 0 {
			out = append(out, fmt.Sprintf("rule %q: From %v matches no package, so it checks nothing", r.Name, r.From))
		}
	}
	sort.Strings(out)
	return out
}

func violation(g Graph, r Rule, pkg string) string {
	if r.Direct {
		for _, imp := range sorted(g[pkg]) {
			if matchAny(r.Forbid, imp) {
				return fmt.Sprintf("rule %q: %s imports %s directly. %s", r.Name, pkg, imp, r.Why)
			}
		}
		return ""
	}
	chain := shortestChain(g, pkg, r.Forbid)
	if chain == nil {
		return ""
	}
	return fmt.Sprintf("rule %q: %s reaches forbidden %s via %s. %s",
		r.Name, pkg, chain[len(chain)-1], strings.Join(chain, " -> "), r.Why)
}

// shortestChain does a breadth-first walk with sorted neighbours, so the
// reported chain is the shortest and is the same on every run.
func shortestChain(g Graph, from string, forbid []string) []string {
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, imp := range sorted(g[cur]) {
			if _, seen := prev[imp]; seen {
				continue
			}
			prev[imp] = cur
			if matchAny(forbid, imp) {
				var chain []string
				for p := imp; p != ""; p = prev[p] {
					chain = append([]string{p}, chain...)
				}
				return chain
			}
			queue = append(queue, imp)
		}
	}
	return nil
}

func matchAny(pats []string, path string) bool {
	for _, p := range pats {
		if match(p, path) {
			return true
		}
	}
	return false
}

func match(pat, path string) bool {
	if prefix, ok := strings.CutSuffix(pat, "/..."); ok {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return pat == path
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}

func sortedKeys(g Graph) []string {
	keys := make([]string, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
