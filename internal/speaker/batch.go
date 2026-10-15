package speaker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iksnerd/ava/pkg/stt/captions"
)

// ManifestName is the file a batch writes last, listing what it made. Its
// presence means the batch finished.
const ManifestName = "manifest.json"

// ManifestEntry is one line of a batch: the WAV it became and where its
// segments sit in it. Line is the line's number in the script, blank lines
// counted, so a scene can be traced back to where it was written.
type ManifestEntry struct {
	Line       int                `json:"line"`
	File       string             `json:"file"`
	Text       string             `json:"text"`
	DurationMS int                `json:"duration_ms"`
	Segments   []captions.Segment `json:"segments"`
}

type scriptLine struct {
	number int
	text   string
}

// scriptLines are the non-blank lines of text, trimmed, each with its number
// in the script.
func scriptLines(text string) []scriptLine {
	var lines []scriptLine
	for i, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, scriptLine{number: i + 1, text: l})
		}
	}
	return lines
}

// batchName is the WAV name for the nth of total lines. At least three
// digits, and wide enough for every number, so the files sort as written.
func batchName(n, total int) string {
	width := max(3, len(strconv.Itoa(total)))
	return fmt.Sprintf("%0*d.wav", width, n)
}

// renderBatch speaks every line of text into its own WAV in opts.OutDir and
// writes a manifest of them. Everything is rendered before anything is
// written, so a failure on line 7 leaves no files rather than six scenes beside
// a manifest that is not there. Files with the same names are overwritten,
// since regenerating a script is the point.
func (s *Speaker) renderBatch(text string, opts Options) error {
	if opts.Out != "" || opts.Timings != "" {
		return errors.New("a batch writes its own files: out and timings do not go with out_dir")
	}
	lines := scriptLines(text)
	if len(lines) == 0 {
		return errors.New("nothing to speak: the script has no lines")
	}

	settings := s.Settings()
	if err := s.ready(settings); err != nil {
		return fmt.Errorf("cannot write %s: %w", opts.OutDir, err)
	}

	manifest := make([]ManifestEntry, len(lines))
	audio := make([][]byte, len(lines))
	for i, l := range lines {
		timed, err := s.Engine.SpeakTimed(s.request(l.text, opts, settings))
		if err != nil {
			return fmt.Errorf("line %d (%q): %w", l.number, l.text, err)
		}
		duration := 0
		if n := len(timed.Segments); n > 0 {
			duration = timed.Segments[n-1].EndMS
		}
		audio[i] = timed.Audio
		manifest[i] = ManifestEntry{
			Line:       l.number,
			File:       batchName(i+1, len(lines)),
			Text:       l.text,
			DurationMS: duration,
			Segments:   timed.Segments,
		}
	}

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.MkdirAll(opts.OutDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", opts.OutDir, err)
	}
	var written []string
	fail := func(err error) error {
		for _, f := range written {
			_ = os.Remove(f)
		}
		return err
	}
	for i, e := range manifest {
		path := filepath.Join(opts.OutDir, e.File)
		if err := writeFile(path, audio[i]); err != nil {
			return fail(err)
		}
		written = append(written, path)
	}
	// The manifest goes last: it is the sign the batch finished.
	if err := writeFile(filepath.Join(opts.OutDir, ManifestName), append(raw, '\n')); err != nil {
		return fail(err)
	}
	return nil
}
