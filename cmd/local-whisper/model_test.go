package main

import "testing"

func TestValidateModel(t *testing.T) {
	cases := []struct {
		model   string
		wantErr bool
	}{
		{"base", false},
		{"tiny", false},
		{"large", true},
		{"", true},
	}
	for _, tc := range cases {
		err := validateModel(tc.model)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateModel(%q) error = %v, wantErr %v", tc.model, err, tc.wantErr)
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
