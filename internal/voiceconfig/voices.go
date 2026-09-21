package voiceconfig

import "strings"

// Voice is one Kokoro voice offered by mlx-engine's /speak.
type Voice struct {
	ID          string
	Description string
}

// voiceIDs are the voices AvaMenuBar's picker offers, which all ship
// in the same Kokoro-82M download — there's nothing extra to fetch to use
// any of them. Kept in sync with VoiceSettings.swift by
// TestVoicesMatchMenuBarAppList.
//
// A comma-separated pair ("af_heart,af_sky") is also valid anywhere a voice
// id is: Kokoro averages the two embeddings into a blended voice.
var voiceIDs = []string{
	"af_alloy", "af_aoede", "af_bella", "af_heart", "af_jessica", "af_kore",
	"af_nicole", "af_nova", "af_river", "af_sarah", "af_sky",
	"am_adam", "am_echo", "am_eric", "am_fenrir", "am_liam", "am_michael",
	"am_onyx", "am_puck", "am_santa",
	"bf_alice", "bf_emma", "bf_isabella", "bf_lily",
	"bm_daniel", "bm_fable", "bm_george", "bm_lewis",
}

// Voices lists the available voices with a human description. The id's
// prefix is the whole story: first letter is the locale (which mlx-engine
// also uses server-side to pick the right phonemizer, so a British voice is
// phonemized with British rules), second is the gender.
func Voices() []Voice {
	out := make([]Voice, 0, len(voiceIDs))
	for _, id := range voiceIDs {
		out = append(out, Voice{ID: id, Description: describe(id)})
	}
	return out
}

func describe(id string) string {
	prefix, _, _ := strings.Cut(id, "_")
	if len(prefix) != 2 {
		return "unknown"
	}

	accent := map[byte]string{
		'a': "American English",
		'b': "British English",
		'e': "Spanish",
		'f': "French",
		'h': "Hindi",
		'i': "Italian",
		'j': "Japanese",
		'p': "Brazilian Portuguese",
		'z': "Mandarin Chinese",
	}[prefix[0]]
	if accent == "" {
		accent = "unknown accent"
	}

	gender := "female"
	if prefix[1] == 'm' {
		gender = "male"
	}
	return accent + ", " + gender
}
