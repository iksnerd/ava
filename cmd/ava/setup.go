package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/iksnerd/ava/internal/enginedist"
	"github.com/iksnerd/ava/pkg/mlx"
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
	var skipEngine, check, asJSON bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install the dependencies, model and TTS engine the binary needs",
		Long: "Installs what ava needs beyond the binary itself:\n" +
			"  1. sox, whisper-cli and uv, via Homebrew\n" +
			"  2. the whisper.cpp base.en model (~141 MB)\n" +
			"  3. the Kokoro TTS engine (~1.2 GB of Python)\n" +
			"  4. the Kokoro model and all its voices (339 MB), so speech needs no\n" +
			"     network afterwards\n\n" +
			"About 1.7 GB in all, most of it the engine and its model.\n\n" +
			"Every step is skipped if it is already done, so re-running is cheap " +
			"and is how you upgrade the engine bundle after installing a new binary.\n\n" +
			"Use --skip-engine for dictation only, without steps 3 and 4. --check\n" +
			"reports what is missing and installs nothing; it exits non-zero until\n" +
			"setup is complete, or while an engine older than this ava is running.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if check {
				return checkSetup(out, skipEngine, asJSON)
			}
			if asJSON {
				return fmt.Errorf("--json goes with --check")
			}

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
	cmd.Flags().BoolVar(&check, "check", false,
		"Report what is missing without installing anything")
	cmd.Flags().BoolVar(&asJSON, "json", false,
		`With --check, print {"ok":bool,"missing":[...],"stale_engine":bool} on stdout`)
	cmd.Flags().BoolVar(&skipEngine, "skip-engine", false,
		"Skip the Kokoro TTS engine and model (most of the ~1.7 GB); speech falls back to the macOS say voice")
	return cmd
}

// setupTools maps each external binary setup installs to its Homebrew
// formula: sox and whisper-cli for dictation, and uv, which the engine step
// needs to resolve its Python environment.
var setupToolFormulae = map[string]string{"sox": "sox", "whisper-cli": "whisper-cpp", "uv": "uv"}

// setupTools installs the external binaries in setupToolFormulae.
func setupTools(out io.Writer) error {
	missing := map[string]string{} // binary -> brew formula
	for bin, formula := range setupToolFormulae {
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
		// anyone who installs packages another way. The menu bar app's Set up
		// card matches "Homebrew is not installed" to offer brew.sh instead,
		// so keep that phrase if this is reworded.
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

// setupEngineURL is where --check looks for a running engine; empty means the
// default. A variable so tests can point it at a stub.
var setupEngineURL string

// staleEngineLine is what --check prints for a running engine older than this
// binary. The menu bar app matches its prefix to tell it from a missing
// install, so keep the two in step.
const staleEngineLine = "❌ The running mlx-engine is older than this ava; " +
	"restart it with `ava engine stop && ava engine start`"

// checkSetup reports what `ava setup` would still install, using the same
// tests setup uses to skip a step, and installs nothing. It also reports a
// running engine whose protocol is older than this binary's, which no install
// step fixes: every speak would fail with it. The menu bar app runs this at
// every start to decide whether to offer Set up; it must not offer one for a
// stale engine (see SetupController.swift).
func checkSetup(out io.Writer, skipEngine, asJSON bool) error {
	missing := []string{}
	tools := keysOf(setupToolFormulae)
	sort.Strings(tools)
	for _, bin := range tools {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("%s (brew install %s)", bin, setupToolFormulae[bin]))
		}
	}
	if home, err := os.UserHomeDir(); err != nil ||
		!modelPresent(filepath.Join(home, modelDirRel, whisperModels["base"].file), whisperModels["base"]) {
		missing = append(missing, "the whisper speech model")
	}
	stale := false
	if !skipEngine && runtime.GOARCH == "arm64" {
		if script, ok := installedEngine(); !ok {
			missing = append(missing, "the Kokoro engine", "the Kokoro model")
		} else if exec.Command("bash", script, "fetch", "--check").Run() != nil {
			// The engine answers for its own model: only it knows the
			// revision it pins and where the Hugging Face cache keeps it.
			missing = append(missing, "the Kokoro model")
		}
	}
	if !skipEngine {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		stale = mlx.NewClient(setupEngineURL).Check(ctx) == mlx.Stale
		cancel()
	}

	ok := len(missing) == 0 && !stale
	if asJSON {
		b, _ := json.Marshal(struct {
			OK      bool     `json:"ok"`
			Missing []string `json:"missing"`
			Stale   bool     `json:"stale_engine"`
		}{ok, missing, stale})
		fmt.Fprintln(out, string(b))
		if ok {
			return nil
		}
		return silentExit{}
	}
	if ok {
		fmt.Fprintln(out, "✅ Ava is set up.")
		return nil
	}
	for _, m := range missing {
		fmt.Fprintf(out, "❌ Missing: %s\n", m)
	}
	if stale {
		fmt.Fprintln(out, staleEngineLine)
	}
	if len(missing) == 0 {
		return silentExit{}
	}
	return fmt.Errorf("setup is incomplete; run `ava setup` to install what is missing")
}

// silentExit is an error that exits non-zero without Execute printing
// anything: for a command that has already said what it has to, or has
// to stay quiet (engine status --check).
type silentExit struct{}

func (silentExit) Error() string { return "" }

func isSilentExit(err error) bool {
	var s silentExit
	return errors.As(err, &s)
}

// installedEngine returns the control script of the bundle `ava setup`
// installs, when its Python environment has been resolved too.
func installedEngine() (string, bool) {
	script, ok := enginedist.InstalledScript()
	if !ok {
		return "", false
	}
	root := filepath.Dir(filepath.Dir(script))
	if _, err := os.Stat(filepath.Join(root, "mlx-engine", ".venv", "bin", "uvicorn")); err != nil {
		return "", false
	}
	return script, true
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

	// Through the bundle just installed, not whichever script `ava engine`
	// would resolve: the fetch must fill the cache this engine will read.
	fetch := exec.Command("bash", filepath.Join(dir, enginedist.ScriptRelPath), "fetch")
	fetch.Stdout, fetch.Stderr = out, os.Stderr
	if err := fetch.Run(); err != nil {
		return fmt.Errorf("the Kokoro model did not download (%w); re-run `ava setup` to retry, "+
			"the engine itself is already installed", err)
	}
	fmt.Fprintln(out, "✅ Kokoro model downloaded.")
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
