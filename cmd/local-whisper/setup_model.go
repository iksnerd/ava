package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// modelDirRel is where whisper.cpp models live, relative to $HOME. It matches
// scripts/setup-model.sh and the path cmd/local-whisper resolves at run time.
const modelDirRel = ".local/share/whisper-cpp"

// whisperModelRevision pins the ggerganov/whisper.cpp Hugging Face commit the
// models are fetched from. `resolve/main` is a mutable ref: whatever is there
// on the day is what gets installed, and without a checksum nothing would
// notice if it changed. Pinning the revision is what makes the sha256s in
// whisperModels checkable at all.
const whisperModelRevision = "5359861c739e955e79d9a303bcbc70fb988958b1"

const whisperModelRepo = "https://huggingface.co/ggerganov/whisper.cpp/resolve/"

// whisperModel is one downloadable model: its filename under modelDirRel and
// what the pinned revision serves for it.
type whisperModel struct {
	file   string
	size   int64
	sha256 string
}

// whisperModels maps the --model flag to what `setup-model` installs. Sizes
// and hashes are the LFS metadata of whisperModelRevision, and match
// `shasum -a 256` of the files the old unpinned download produced.
var whisperModels = map[string]whisperModel{
	"base": {baseModel, 147964211, "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002"},
	"tiny": {tinyModel, 77704715, "921e4cf8686fdd993dcd081a5da5b6c365bfde1162e72b08d75ac75289920b1f"},
}

func modelURL(m whisperModel) string {
	return whisperModelRepo + whisperModelRevision + "/" + m.file
}

// newSetupModelCmd downloads one whisper.cpp model and nothing else. `setup`
// does this for base.en as one of its three steps; this is the command to
// reach for when the model is the only thing missing, or for the tiny model,
// which `setup` does not fetch.
func newSetupModelCmd() *cobra.Command {
	var model string

	cmd := &cobra.Command{
		Use:   "setup-model",
		Short: "Download a whisper.cpp model (base or tiny)",
		Long: "Downloads a whisper.cpp model to ~/" + modelDirRel + "/, from a pinned\n" +
			"revision, and verifies its sha256 before installing it.\n\n" +
			"Skipped if the model is already there, so re-running is cheap.",
		Example: "  local-whisper setup-model\n" +
			"  local-whisper setup-model --model tiny",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateModel(model); err != nil {
				return err
			}
			return installModel(cmd.OutOrStdout(), model)
		},
	}
	cmd.Flags().StringVar(&model, "model", "base", "Model size: base or tiny")
	_ = cmd.RegisterFlagCompletionFunc("model",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"base", "tiny"}, cobra.ShellCompDirectiveNoFileComp
		})
	return cmd
}

// installModel installs the named model into ~/.local/share/whisper-cpp.
func installModel(out io.Writer, name string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	m := whisperModels[name]
	return downloadModel(out, filepath.Join(home, modelDirRel), m, modelURL(m))
}

// downloadModel fetches m from url into dir unless a file of the expected size
// is already there. Split from installModel so tests can point it at a local
// server and a fake model.
func downloadModel(out io.Writer, dir string, m whisperModel, url string) error {
	path := filepath.Join(dir, m.file)

	if info, err := os.Stat(path); err == nil {
		// Size, not a full hash: a truncated download is the failure that
		// actually happens, and the size catches it without reading 141 MB
		// on every re-run.
		if info.Size() == m.size {
			fmt.Fprintf(out, "✅ Model already present (%s, %s)\n", m.file, humanBytes(info.Size()))
			return nil
		}
		fmt.Fprintf(out, "⚠️  %s is %s, expected %s — downloading it again\n",
			path, humanBytes(info.Size()), humanBytes(m.size))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	fmt.Fprintf(out, "⬇️  Downloading %s (~%s)...\n", m.file, humanBytes(m.size))
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download model: %s returned %s", url, resp.Status)
	}

	// Download to a temp file in the same directory and rename, so an
	// interrupted run cannot leave a truncated model that looks installed —
	// whisper-cli would then fail with a parse error nowhere near the cause.
	tmp, err := os.CreateTemp(dir, ".model-*.partial")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != m.sha256 {
		return fmt.Errorf("download model: %s has sha256 %s, expected %s (%s received); "+
			"nothing was installed", m.file, got, m.sha256, humanBytes(n))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	fmt.Fprintf(out, "✅ Model installed (%s, sha256 verified)\n", humanBytes(n))
	return nil
}
