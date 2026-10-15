// Package captions renders timed transcript segments as SRT, WebVTT or JSON.
// It is pure: the segments come from pkg/stt/whisper, and nothing here reads
// a file or runs a program, so the formats can be checked without whisper-cli.
package captions

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Segment is one timed stretch of speech. Offsets are milliseconds from the
// start of the audio, which is what whisper-cli reports.
type Segment struct {
	StartMS int    `json:"start_ms"`
	EndMS   int    `json:"end_ms"`
	Text    string `json:"text"`
}

// SRT renders numbered cues with a comma before the milliseconds.
func SRT(segments []Segment) string {
	return cues(segments, ",", "", true)
}

// VTT renders a WebVTT document: a header, and a dot before the milliseconds.
func VTT(segments []Segment) string {
	return cues(segments, ".", "WEBVTT\n\n", false)
}

// JSON renders the segments as an array of start_ms, end_ms and text.
// No segments render as an empty string, like SRT and VTT, so a caller can
// tell "no speech" from an empty document.
func JSON(segments []Segment) (string, error) {
	segments = speech(segments)
	if len(segments) == 0 {
		return "", nil
	}
	out, err := json.Marshal(segments)
	if err != nil {
		return "", fmt.Errorf("encode segments: %w", err)
	}
	return string(out) + "\n", nil
}

// speech trims every segment's text and drops the ones left blank, which
// would show as an empty caption.
func speech(segments []Segment) []Segment {
	var out []Segment
	for _, s := range segments {
		if s.Text = strings.TrimSpace(s.Text); s.Text != "" {
			out = append(out, s)
		}
	}
	return out
}

// cues is the one cue layout SRT and VTT share; they differ in the
// millisecond separator, a leading header, and whether a cue is numbered.
func cues(segments []Segment, msSep, header string, numbered bool) string {
	segments = speech(segments)
	if len(segments) == 0 {
		return ""
	}
	blocks := make([]string, len(segments))
	for i, s := range segments {
		var b strings.Builder
		if numbered {
			fmt.Fprintf(&b, "%d\n", i+1)
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n", clock(s.StartMS, msSep), clock(s.EndMS, msSep), s.Text)
		blocks[i] = b.String()
	}
	return header + strings.Join(blocks, "\n")
}

func clock(ms int, msSep string) string {
	return fmt.Sprintf("%02d:%02d:%02d%s%03d",
		ms/3600000, ms/60000%60, ms/1000%60, msSep, ms%1000)
}
