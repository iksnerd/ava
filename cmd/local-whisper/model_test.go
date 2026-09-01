package main

import "testing"

func TestValidateEngine(t *testing.T) {
	cases := []struct {
		engine  string
		wantErr bool
	}{
		{"whisper", false},
		{"voxtral", false},
		{"", true},
		{"WHISPER", true},
		{"gpt4", true},
	}
	for _, tc := range cases {
		err := validateEngine(tc.engine)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateEngine(%q) error = %v, wantErr %v", tc.engine, err, tc.wantErr)
		}
	}
}

func TestValidateModel(t *testing.T) {
	cases := []struct {
		engine, model string
		wantErr       bool
	}{
		{"whisper", "base", false},
		{"whisper", "tiny", false},
		{"whisper", "large", true},
		{"whisper", "", true},
		// The model flag only constrains the whisper engine.
		{"voxtral", "large", false},
		{"voxtral", "", false},
	}
	for _, tc := range cases {
		err := validateModel(tc.engine, tc.model)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateModel(%q, %q) error = %v, wantErr %v", tc.engine, tc.model, err, tc.wantErr)
		}
	}
}

func TestSelectModelFile(t *testing.T) {
	cases := []struct {
		modelName string
		want      string
	}{
		{"tiny", tinyModel},
		{"base", baseModel},
		{"", baseModel},
		{"anything-else", baseModel},
	}
	for _, tc := range cases {
		if got := selectModelFile(tc.modelName); got != tc.want {
			t.Errorf("selectModelFile(%q) = %q, want %q", tc.modelName, got, tc.want)
		}
	}
}
