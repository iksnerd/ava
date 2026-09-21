// Package a11y turns a Chrome accessibility tree into what a screen reader
// would say, so a page can be listened to rather than only read.
//
// The input is chrome-devtools MCP's take_snapshot output, which is
// Chrome's own accessibility tree (Puppeteer's page.accessibility.snapshot)
// serialized one node per line:
//
//	uid=1_0 RootWebArea "Acme Dashboard"
//	  uid=1_1 navigation "Main"
//	    uid=1_2 link "Home" focusable
//	    uid=1_3 heading "Overview" level="1"
//
// Everything here is pure: parse, render, inspect. Speaking the result is
// internal/speaker's job, and fetching the snapshot is the MCP client's.
// That split is deliberate — the rendering has to be identical from one run
// to the next for a spoken audit to be worth anything, so it's ordinary
// tested code rather than something re-improvised per session.
//
// Scope stops at role + name + state. Live regions, table navigation and a
// virtual cursor are real screen-reader behaviour this does not model.
package a11y

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is one line of the snapshot.
type Node struct {
	UID   string
	Role  string
	Name  string
	Depth int
	// Attrs holds both `key="value"` attributes and bare boolean flags
	// (stored as "true"), exactly as the snapshot spelled them.
	Attrs map[string]string
}

// Mode selects what to announce. Screen-reader users navigate by heading,
// link and landmark far more than they read a page top to bottom, so each
// of those is its own pass.
type Mode string

const (
	// ModeReading announces every meaningful node in document order.
	ModeReading Mode = "reading"
	// ModeHeadings is the heading outline.
	ModeHeadings Mode = "headings"
	// ModeLinks is every link, in order, which is how "read more" ×6 shows up.
	ModeLinks Mode = "links"
	// ModeLandmarks is the page's regions.
	ModeLandmarks Mode = "landmarks"
	// ModeForms is form controls with their labels and states.
	ModeForms Mode = "forms"
)

// Modes lists every supported mode, for help text and tool schemas.
func Modes() []string {
	return []string{string(ModeReading), string(ModeHeadings), string(ModeLinks), string(ModeLandmarks), string(ModeForms)}
}

// IsMode reports whether m is one of Modes(). Announce deliberately falls
// back to reading the whole page for anything else — silence is the one
// useless answer to give an agent — so a caller that *can* report an error,
// like the CLI, checks here first rather than letting a typo quietly change
// what it does.
func IsMode(m Mode) bool {
	for _, known := range Modes() {
		if string(m) == known {
			return true
		}
	}
	return false
}

// selectedSuffix is appended by chrome-devtools to whichever node is
// selected in the DevTools Elements panel. It's UI state, not page content.
const selectedSuffix = " [selected in the DevTools Elements panel]"

// Parse reads a take_snapshot payload. Lines that aren't tree nodes (the
// surrounding prose of an MCP response, blank lines) are skipped rather
// than treated as errors — callers paste the whole tool output.
func Parse(snapshot string) []Node {
	var nodes []Node
	for _, line := range strings.Split(snapshot, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSuffix(line, selectedSuffix)

		trimmed := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(trimmed, "uid=") {
			continue
		}

		node := Node{
			Depth: (len(line) - len(trimmed)) / 2,
			Attrs: map[string]string{},
		}
		for i, tok := range tokenize(trimmed) {
			switch {
			case i == 0:
				node.UID = strings.TrimPrefix(tok, "uid=")
			case strings.HasPrefix(tok, `"`):
				node.Name = strings.Trim(tok, `"`)
			case i == 1:
				node.Role = tok
			default:
				if key, value, ok := strings.Cut(tok, "="); ok {
					node.Attrs[key] = strings.Trim(value, `"`)
				} else {
					node.Attrs[tok] = "true"
				}
			}
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// tokenize splits on spaces but keeps quoted spans intact, so an accessible
// name like "Save and close" survives as one token.
func tokenize(line string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false

	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ' ' && !inQuotes:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// Announce renders nodes as utterances, one per line of speech. An
// unrecognized mode reads the whole page rather than announcing nothing —
// silence is the one useless answer here.
func Announce(nodes []Node, mode Mode) []string {
	var out []string
	for _, n := range nodes {
		var keep bool
		switch mode {
		case ModeHeadings:
			keep = n.Role == "heading"
		case ModeLinks:
			keep = n.Role == "link"
		case ModeLandmarks:
			keep = isLandmark(n.Role)
		case ModeForms:
			keep = isFormControl(n.Role)
		default:
			keep = true
		}
		if !keep {
			continue
		}
		if utterance := announce(n); utterance != "" {
			out = append(out, utterance)
		}
	}
	return out
}

// roleNames maps ARIA roles to how a screen reader says them out loud.
var roleNames = map[string]string{
	"RootWebArea":   "web page",
	"heading":       "heading",
	"link":          "link",
	"button":        "button",
	"textbox":       "edit text",
	"searchbox":     "search text field",
	"combobox":      "combo box",
	"checkbox":      "checkbox",
	"radio":         "radio button",
	"switch":        "switch",
	"slider":        "slider",
	"image":         "image",
	"img":           "image",
	"list":          "list",
	"listitem":      "list item",
	"table":         "table",
	"tab":           "tab",
	"menuitem":      "menu item",
	"dialog":        "dialog",
	"alert":         "alert",
	"navigation":    "navigation",
	"banner":        "banner",
	"contentinfo":   "content information",
	"complementary": "complementary",
	"region":        "region",
	"search":        "search",
	"form":          "form",
	"main":          "main",
	"article":       "article",
}

// silentRoles carry no meaning of their own — a screen reader passes
// straight through them to their children.
var silentRoles = map[string]bool{
	"generic":          true,
	"genericContainer": true,
	"none":             true,
	"ignored":          true,
	"presentation":     true,
	"InlineTextBox":    true,
	"LineBreak":        true,
}

// textRoles are spoken as their bare content, with no role prefix.
var textRoles = map[string]bool{
	"StaticText": true,
	"text":       true,
	"paragraph":  true,
	"emphasis":   true,
	"strong":     true,
	"code":       true,
}

func announce(n Node) string {
	if silentRoles[n.Role] {
		return ""
	}
	if textRoles[n.Role] {
		return n.Name
	}

	spoken, known := roleNames[n.Role]
	if !known {
		if n.Name == "" {
			return ""
		}
		spoken = n.Role
	}

	if n.Role == "heading" {
		if level := n.Attrs["level"]; level != "" {
			spoken = "heading level " + level
		}
	}

	parts := []string{spoken}
	switch {
	case n.Name != "":
		parts = append(parts, n.Name)
	case needsName(n.Role):
		// The whole reason to listen to a page: an unlabeled control is
		// silent in a text report and a dead end when heard.
		parts = append(parts, "unlabeled")
	}

	parts = append(parts, states(n)...)
	return strings.Join(parts, ", ")
}

// states appends what a screen reader says after the name.
func states(n Node) []string {
	var out []string
	for _, flag := range []struct{ attr, spoken string }{
		{"required", "required"},
		{"invalid", "invalid"},
		{"disabled", "dimmed"},
		{"readonly", "read only"},
	} {
		if v, ok := n.Attrs[flag.attr]; ok && v != "false" {
			out = append(out, flag.spoken)
		}
	}
	if v, ok := n.Attrs["checked"]; ok {
		if v == "false" {
			out = append(out, "unchecked")
		} else {
			out = append(out, "checked")
		}
	}
	if v, ok := n.Attrs["expanded"]; ok {
		if v == "false" {
			out = append(out, "collapsed")
		} else {
			out = append(out, "expanded")
		}
	}
	return out
}

func isLandmark(role string) bool {
	switch role {
	case "navigation", "main", "banner", "contentinfo", "complementary", "region", "search", "form":
		return true
	}
	return false
}

func isFormControl(role string) bool {
	switch role {
	case "textbox", "searchbox", "combobox", "checkbox", "radio", "switch", "slider", "button", "listbox", "spinbutton":
		return true
	}
	return false
}

// needsName is true for roles that are useless without an accessible name:
// a control you can reach but can't identify.
func needsName(role string) bool {
	if isFormControl(role) {
		return true
	}
	switch role {
	case "link", "image", "img", "tab", "menuitem":
		return true
	}
	return false
}

// Findings reports the problems that are obvious once a page is heard in
// order. It is not an axe replacement — run chrome-devtools'
// lighthouse_audit for the rule-based pass; this covers what that report
// can't tell you about how the page sounds.
func Findings(nodes []Node) []string {
	var out []string

	for _, n := range nodes {
		if n.Name == "" && needsName(n.Role) {
			out = append(out, fmt.Sprintf("uid=%s: %s with no accessible name — announced as %q", n.UID, n.Role, announce(n)))
		}
	}

	out = append(out, duplicateLinkFindings(nodes)...)
	out = append(out, headingFindings(nodes)...)
	return out
}

func duplicateLinkFindings(nodes []Node) []string {
	counts := map[string]int{}
	var order []string
	for _, n := range nodes {
		if n.Role != "link" || n.Name == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(n.Name))
		if counts[key] == 0 {
			order = append(order, n.Name)
		}
		counts[key]++
	}

	var out []string
	for _, name := range order {
		if c := counts[strings.ToLower(strings.TrimSpace(name))]; c > 1 {
			out = append(out, fmt.Sprintf("%d links announce as %q — indistinguishable in a link-list pass", c, name))
		}
	}
	return out
}

func headingFindings(nodes []Node) []string {
	var out []string
	levels := map[int]bool{}
	previous := 0

	for _, n := range nodes {
		if n.Role != "heading" {
			continue
		}
		level, err := strconv.Atoi(n.Attrs["level"])
		if err != nil {
			continue
		}
		levels[level] = true
		if previous != 0 && level > previous+1 {
			out = append(out, fmt.Sprintf("uid=%s: heading level jumps from %d to %d — skipped levels break outline navigation", n.UID, previous, level))
		}
		previous = level
	}

	if !levels[1] {
		out = append(out, "page has no level 1 heading — nothing anchors the outline")
	}

	return out
}
