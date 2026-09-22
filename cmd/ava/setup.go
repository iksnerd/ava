package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/enginedist"
)

// newSetupCmd installs everything ava needs that is not the binary.
//
// It exists because the binary is otherwise only half usable on a machine
// without a checkout: dictation needs sox, whisper-cli and a model, and Kokoro
// TTS needs the Python engine that lives in the repo. Without this, `speak`
// silently falls back to the macOS `say` voice and the project's headline
// feature never runs. `make setup` does the same three things, but only from
// a checkout — which is the one thing a downloaded binary does not have.
func newSetupCmd() *cobra.Command {
	var skipEngine bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install the dependencies, model and TTS engine the binary needs",
		Long: "Installs what ava needs beyond the binary itself:\n" +
			"  1. sox and whisper-cli, via Homebrew\n" +
			"  2. the whisper.cpp base.en model (~141 MB)\n" +
			"  3. the Kokoro TTS engine (~1.2 GB of Python, plus a 339 MB model\n" +
			"     the server fetches on first use)\n\n" +
			"Every step is skipped if it is already done, so re-running is cheap " +
			"and is how you upgrade the engine bundle after installing a new binary.\n\n" +
			"Use --skip-engine for dictation only, without the 1.2 GB.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if err := setupTools(out); err != nil {
				return err
			}
			if err := installModel(out, "base"); err != nil {
				return err
			}
			if skipEngine {
				fmt.Fprintln(out, "⏭️  Skipping the Kokoro engine (--skip-engine).")
				fmt.Fprintln(out, "   Speech will use the macOS `say` voice.")
			} else if err := setupEngine(out); err != nil {
				return err
			}

			// Suggest something that exercises what was just installed:
			// after --skip-engine, `speak` would only demo the fallback voice.
			if skipEngine {
				fmt.Fprintln(out, "\n✅ Setup complete. Try: ava   (speak, then pause to finish)")
			} else {
				fmt.Fprintln(out, "\n✅ Setup complete. Try: ava speak \"hello\"")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipEngine, "skip-engine", false,
		"Skip the Kokoro TTS engine (~1.2 GB); speech falls back to the macOS say voice")
	return cmd
}

// setupTools installs the two external binaries dictation shells out to.
func setupTools(out io.Writer) error {
	missing := map[string]string{} // binary -> brew formula
	for bin, formula := range map[string]string{"sox": "sox", "whisper-cli": "whisper-cpp"} {
		if _, err := exec.LookPath(bin); err != nil {
			missing[bin] = formula
		} else {
			fmt.Fprintf(out, "✅ %s already installed\n", bin)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	if _, err := exec.LookPath("brew"); err != nil {
		// Naming the formulae matters: without them this is a dead end for
		// anyone who installs packages another way.
		var formulae []string
		for _, f := range missing {
			formulae = append(formulae, f)
		}
		return fmt.Errorf("missing %s, and Homebrew is not installed to get them.\n"+
			"   Install Homebrew (https://brew.sh) and re-run, or install these yourself: %s",
			strings.Join(keysOf(missing), " and "), strings.Join(formulae, " "))
	}

	for bin, formula := range missing {
		fmt.Fprintf(out, "⬇️  Installing %s (brew install %s)...\n", bin, formula)
		c := exec.Command("brew", "install", formula)
		c.Stdout, c.Stderr = out, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("brew install %s: %w", formula, err)
		}
	}
	return nil
}

func keysOf(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// setupEngine materializes the embedded bundle and resolves its Python
// dependencies.
func setupEngine(out io.Writer) error {
	if runtime.GOARCH != "arm64" {
		// Matching scripts/setup-deps.sh: MLX has no Intel build, and failing
		// the whole setup at the last step after everything that mattered
		// succeeded is worse than saying so.
		fmt.Fprintln(out, "⏭️  Skipping the Kokoro engine (Apple Silicon only — MLX has no Intel build).")
		fmt.Fprintln(out, "   Dictation is fully set up. Speech will use the macOS `say` voice.")
		return nil
	}

	dir, err := enginedist.DefaultDir()
	if err != nil {
		return err
	}
	if err := checkPathBudget(dir); err != nil {
		return err
	}

	fmt.Fprintf(out, "⬇️  Installing the Kokoro engine into %s\n", dir)
	n, err := enginedist.Materialize(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "   Unpacked %d files.\n", n)

	if _, err := exec.LookPath("uv"); err != nil {
		return fmt.Errorf("the engine needs `uv` to resolve its Python dependencies, "+
			"and it is not on PATH.\n"+
			"   Install it (https://docs.astral.sh/uv/) and re-run `ava setup`.\n"+
			"   The bundle is already unpacked at %s, so the re-run only does this step", dir)
	}

	fmt.Fprintln(out, "   Resolving Python dependencies (~1.2 GB, a few minutes the first time)...")
	c := exec.Command("uv", "sync")
	c.Dir = filepath.Join(dir, "mlx-engine")
	c.Stdout, c.Stderr = out, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("uv sync in %s: %w", c.Dir, err)
	}

	fmt.Fprintln(out, "✅ Kokoro engine installed.")
	fmt.Fprintln(out, "   The 339 MB voice model downloads on the first `ava speak`.")
	return nil
}

// espeakDataSuffix is what uv adds under the engine directory before
// espeak-ng's own data path begins.
const espeakDataSuffix = "/mlx-engine/.venv/lib/python3.11/site-packages/espeakng_loader/espeak-ng-data"

// espeakPathBudget is espeak-ng's fixed buffer for its data path (N_PATH_HOME,
// 160 bytes). Past it the path is truncated, and the server dies reporting a
// missing "phontab" against a path that is real up to the point it was cut —
// which names neither the length nor the install directory. Measured: an
// install at a 196-character path failed this way, the same bundle at 91
// characters started fine.
const espeakPathBudget = 160

// checkPathBudget refuses an install directory whose espeak data path would be
// truncated, because the failure it prevents surfaces minutes later, after uv
// has resolved 1.2 GB, and points at espeak rather than at the directory.
func checkPathBudget(dir string) error {
	n := len(dir) + len(espeakDataSuffix)
	if n < espeakPathBudget {
		return nil
	}
	return fmt.Errorf("the install path is too long for espeak-ng: %d characters, "+
		"and it truncates the data path at %d.\n"+
		"   The server would start, load Kokoro, then fail on a missing 'phontab'.\n"+
		"   Install somewhere shorter with %s=/some/short/path, or leave it unset "+
		"to use the default.", n, espeakPathBudget, enginedist.DirEnv)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(n)/float64(div), "KMGT"[exp])
}
