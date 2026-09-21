package a11y

import (
	"reflect"
	"strings"
	"testing"
)

// The shape chrome-devtools MCP's take_snapshot emits (SnapshotFormatter):
// two spaces of indent per level, `uid=<id>`, the ARIA role, the accessible
// name in quotes, then `key="value"` or bare-boolean attributes.
const sampleSnapshot = `uid=1_0 RootWebArea "Acme Dashboard"
  uid=1_1 navigation "Main"
    uid=1_2 link "Home" focusable
    uid=1_3 link "Read more" focusable
  uid=1_4 main
    uid=1_5 heading "Overview" level="1"
    uid=1_6 StaticText "Welcome back."
    uid=1_7 heading "Recent activity" level="3"
    uid=1_8 image
    uid=1_9 link "Read more" focusable
  uid=1_10 form "Sign up"
    uid=1_11 textbox "Email" focusable required
    uid=1_12 button ""
`

func TestParseReadsUIDRoleNameAndDepth(t *testing.T) {
	nodes := Parse(sampleSnapshot)

	if len(nodes) != 13 {
		t.Fatalf("parsed %d nodes, want 13", len(nodes))
	}

	root := nodes[0]
	if root.UID != "1_0" || root.Role != "RootWebArea" || root.Name != "Acme Dashboard" || root.Depth != 0 {
		t.Errorf("root = %+v, want uid=1_0 RootWebArea %q depth 0", root, "Acme Dashboard")
	}

	link := nodes[2]
	if link.UID != "1_2" || link.Role != "link" || link.Name != "Home" || link.Depth != 2 {
		t.Errorf("nodes[2] = %+v, want uid=1_2 link %q depth 2", link, "Home")
	}
}

func TestParseReadsValueAndBooleanAttributes(t *testing.T) {
	nodes := Parse(sampleSnapshot)

	heading := nodes[5]
	if got := heading.Attrs["level"]; got != "1" {
		t.Errorf("level = %q, want %q", got, "1")
	}

	link := nodes[2]
	if _, ok := link.Attrs["focusable"]; !ok {
		t.Errorf("attrs = %v, want a bare `focusable` flag", link.Attrs)
	}
}

func TestParseKeepsSpacesInsideQuotedNames(t *testing.T) {
	nodes := Parse(`uid=2_0 button "Save and close" focusable`)
	if len(nodes) != 1 {
		t.Fatalf("parsed %d nodes, want 1", len(nodes))
	}
	if nodes[0].Name != "Save and close" {
		t.Errorf("name = %q, want %q", nodes[0].Name, "Save and close")
	}
	if _, ok := nodes[0].Attrs["focusable"]; !ok {
		t.Errorf("attrs = %v, want focusable to survive the quoted name", nodes[0].Attrs)
	}
}

// take_snapshot's output is wrapped in prose by the MCP response; anything
// that isn't a `uid=` line is not part of the tree.
func TestParseSkipsNonSnapshotLines(t *testing.T) {
	nodes := Parse("# Page content\n\nuid=1_0 RootWebArea \"Title\"\n\nSome trailing note.\n")
	if len(nodes) != 1 {
		t.Fatalf("parsed %d nodes, want 1", len(nodes))
	}
	if nodes[0].Name != "Title" {
		t.Errorf("name = %q, want %q", nodes[0].Name, "Title")
	}
}

func TestParseHandlesNodesWithNoName(t *testing.T) {
	nodes := Parse(sampleSnapshot)
	if nodes[4].Role != "main" || nodes[4].Name != "" {
		t.Errorf("nodes[4] = %+v, want an unnamed `main`", nodes[4])
	}
}

func TestAnnounceReadingSpeaksRoleThenName(t *testing.T) {
	got := Announce(Parse(sampleSnapshot), ModeReading)
	joined := strings.Join(got, "\n")

	for _, want := range []string{
		"web page, Acme Dashboard",
		"navigation, Main",
		"link, Home",
		"heading level 1, Overview",
		"Welcome back.",
		"edit text, Email, required",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("reading mode missing %q; got:\n%s", want, joined)
		}
	}
}

// The point of hearing a page is catching what a text report doesn't show —
// an unlabeled control announces as its bare role, and that has to be
// audible rather than silently skipped.
func TestAnnounceReadingSurfacesUnlabeledNodes(t *testing.T) {
	joined := strings.Join(Announce(Parse(sampleSnapshot), ModeReading), "\n")
	if !strings.Contains(joined, "image, unlabeled") {
		t.Errorf("want an unlabeled image announced; got:\n%s", joined)
	}
	if !strings.Contains(joined, "button, unlabeled") {
		t.Errorf("want an unlabeled button announced; got:\n%s", joined)
	}
}

func TestAnnounceHeadingsIsAnOutline(t *testing.T) {
	got := Announce(Parse(sampleSnapshot), ModeHeadings)
	want := []string{"heading level 1, Overview", "heading level 3, Recent activity"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %q, want %q", got, want)
	}
}

func TestAnnounceLinks(t *testing.T) {
	got := Announce(Parse(sampleSnapshot), ModeLinks)
	want := []string{"link, Home", "link, Read more", "link, Read more"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %q, want %q", got, want)
	}
}

func TestAnnounceLandmarks(t *testing.T) {
	got := Announce(Parse(sampleSnapshot), ModeLandmarks)
	want := []string{"navigation, Main", "main", "form, Sign up"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("landmarks = %q, want %q", got, want)
	}
}

func TestAnnounceForms(t *testing.T) {
	got := Announce(Parse(sampleSnapshot), ModeForms)
	want := []string{"edit text, Email, required", "button, unlabeled"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("forms = %q, want %q", got, want)
	}
}

func TestAnnounceUnknownModeFallsBackToReading(t *testing.T) {
	nodes := Parse(sampleSnapshot)
	if !reflect.DeepEqual(Announce(nodes, Mode("nonsense")), Announce(nodes, ModeReading)) {
		t.Error("an unknown mode should read the whole page rather than announce nothing")
	}
}

func TestFindings(t *testing.T) {
	got := strings.Join(Findings(Parse(sampleSnapshot)), "\n")

	for _, want := range []string{
		`button with no accessible name`,  // uid=1_12
		`image with no accessible name`,   // uid=1_8
		`2 links announce as "Read more"`, // uid=1_3 and uid=1_9
		`heading level jumps from 1 to 3`, // uid=1_5 -> uid=1_7
	} {
		if !strings.Contains(got, want) {
			t.Errorf("findings missing %q; got:\n%s", want, got)
		}
	}
}

func TestFindingsCleanPageHasNone(t *testing.T) {
	clean := `uid=1_0 RootWebArea "Clean"
  uid=1_1 main
    uid=1_2 heading "Title" level="1"
    uid=1_3 heading "Section" level="2"
    uid=1_4 link "Read the docs" focusable
    uid=1_5 image "A cat asleep on a keyboard"
`
	if got := Findings(Parse(clean)); len(got) != 0 {
		t.Errorf("findings = %q, want none for a clean page", got)
	}
}

func TestFindingsFlagsMissingLevelOneHeading(t *testing.T) {
	noH1 := `uid=1_0 RootWebArea "No h1"
  uid=1_1 main
    uid=1_2 heading "Section" level="2"
`
	got := strings.Join(Findings(Parse(noH1)), "\n")
	if !strings.Contains(got, "no level 1 heading") {
		t.Errorf("findings = %q, want a missing-h1 finding", got)
	}
}

func TestIsMode(t *testing.T) {
	for _, m := range Modes() {
		if !IsMode(Mode(m)) {
			t.Errorf("IsMode(%q) = false, want true", m)
		}
	}
	if IsMode(Mode("nonsense")) {
		t.Error("IsMode(\"nonsense\") = true, want false")
	}
	// Announce deliberately falls back to reading for an unknown mode so an
	// agent never gets silence; callers that can report an error (the CLI)
	// use IsMode to reject a typo instead.
	if IsMode(Mode("")) {
		t.Error("IsMode(\"\") = true, want false — an empty mode is the caller's default to apply")
	}
}
