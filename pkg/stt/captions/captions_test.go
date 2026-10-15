package captions

import (
	"strings"
	"testing"
)

// The expected strings are written out by hand from the SRT and WebVTT
// formats, not built the way the renderer builds them.
var segments = []Segment{
	{StartMS: 0, EndMS: 2500, Text: " Once upon a time."},
	{StartMS: 2500, EndMS: 3723456, Text: "There was a lamp.  "},
}

func TestSRT(t *testing.T) {
	want := "1\n00:00:00,000 --> 00:00:02,500\nOnce upon a time.\n\n" +
		"2\n00:00:02,500 --> 01:02:03,456\nThere was a lamp.\n"
	if got := SRT(segments); got != want {
		t.Errorf("SRT =\n%q\nwant\n%q", got, want)
	}
}

func TestVTTUsesADotAndAHeader(t *testing.T) {
	want := "WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:02.500\nOnce upon a time.\n\n" +
		"00:00:02.500 --> 01:02:03.456\nThere was a lamp.\n"
	if got := VTT(segments); got != want {
		t.Errorf("VTT =\n%q\nwant\n%q", got, want)
	}
}

func TestJSONIsAnArrayOfMillisecondOffsets(t *testing.T) {
	got, err := JSON(segments)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"start_ms":0,"end_ms":2500,"text":"Once upon a time."},` +
		`{"start_ms":2500,"end_ms":3723456,"text":"There was a lamp."}]`
	if strings.TrimSpace(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

// A cue with no words would show as a blank caption, so it is dropped and the
// numbering stays contiguous.
func TestBlankSegmentsAreDropped(t *testing.T) {
	in := []Segment{{0, 1000, "  "}, {1000, 2000, "hi"}}
	if got := SRT(in); got != "1\n00:00:01,000 --> 00:00:02,000\nhi\n" {
		t.Errorf("SRT = %q", got)
	}
}

func TestEmptyInputRendersNothingRatherThanAnEmptyDocument(t *testing.T) {
	if SRT(nil) != "" || VTT(nil) != "" {
		t.Error("no segments should render as an empty string, so callers can report 'no speech'")
	}
	if got, _ := JSON(nil); got != "" {
		t.Errorf("JSON(nil) = %q, want empty", got)
	}
}
