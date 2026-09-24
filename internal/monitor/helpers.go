package monitor

import (
	"fmt"
	"path/filepath"
	"time"
)

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func languageSuffix(language string) string {
	if language == "" || language == "en" {
		return ""
	}
	return ", language: " + language
}

// resolvePythonPath applies the --python flag's default: the venv python3
// under voxtralDir, unless an explicit path was given.
func resolvePythonPath(pythonPath, voxtralDir string) string {
	if pythonPath != "" {
		return pythonPath
	}
	return filepath.Join(voxtralDir, ".venv", "bin", "python3")
}

// defaultLogPath is the --log flag's default: a timestamped file under dir.
func defaultLogPath(dir string, now time.Time) string {
	return filepath.Join(dir, fmt.Sprintf("transcript-%s.txt", now.Format("20060102-150405")))
}
