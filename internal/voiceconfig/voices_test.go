package voiceconfig

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestVoicesDescribeAccentAndGenderFromThePrefix(t *testing.T) {
	byID := map[string]Voice{}
	for _, v := range Voices() {
		byID[v.ID] = v
	}

	for id, want := range map[string]string{
		"af_heart":  "American English, female",
		"am_adam":   "American English, male",
		"bf_emma":   "British English, female",
		"bm_george": "British English, male",
	} {
		got, ok := byID[id]
		if !ok {
			t.Errorf("%s missing from Voices()", id)
			continue
		}
		if got.Description != want {
			t.Errorf("%s description = %q, want %q", id, got.Description, want)
		}
	}
}

func TestDefaultVoiceIsInTheList(t *testing.T) {
	for _, v := range Voices() {
		if v.ID == Defaults().Voice {
			return
		}
	}
	t.Errorf("the default voice %q isn't in Voices()", Defaults().Voice)
}

// The menu bar app's picker and this list are the same set of voices from
// the same Kokoro model, maintained in two languages. This is what catches
// them drifting apart.
func TestVoicesMatchMenuBarAppList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "AvaMenuBar", "Sources", "AvaMenuBar", "VoiceSettings.swift"))
	if err != nil {
		t.Fatalf("read VoiceSettings.swift: %v", err)
	}

	// Only KokoroVoice.english holds the full list; the `var voice =`
	// default declared above it is a single id that would otherwise slip in.
	_, after, found := strings.Cut(string(data), "static let english")
	if !found {
		t.Fatal("no KokoroVoice.english literal in VoiceSettings.swift")
	}
	// Cut again at the opening bracket of the array literal, so the `]` of
	// its `[String]` type annotation isn't mistaken for the list's end.
	_, list, found := strings.Cut(after, "= [")
	if !found {
		t.Fatal("no array literal after KokoroVoice.english")
	}
	end := strings.Index(list, "]")
	if end < 0 {
		t.Fatal("unterminated KokoroVoice.english literal")
	}

	swift := regexp.MustCompile(`"([ab][fm]_[a-z]+)"`).FindAllStringSubmatch(list[:end], -1)
	var want []string
	for _, m := range swift {
		want = append(want, m[1])
	}
	if len(want) == 0 {
		t.Fatal("parsed no voice ids out of VoiceSettings.swift")
	}

	var got []string
	for _, v := range Voices() {
		got = append(got, v.ID)
	}
	sort.Strings(want)
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Voices() = %v\nVoiceSettings.swift = %v\nkeep the two lists in sync", got, want)
	}
}
