package archtest

const module = "github.com/iksnerd/ava"

const (
	cobra  = "github.com/spf13/cobra/..."
	mcpSDK = "github.com/modelcontextprotocol/go-sdk/..."
)

// Rules is the dependency direction of the module, verified against the real
// graph when written. Only what holds today is here.
var Rules = []Rule{
	{
		Name:   "pkg-free-of-adapter-frameworks",
		Why:    "A library consumer must not pay for the CLI or MCP framework; move the framework use into cmd/.",
		From:   []string{module + "/pkg/..."},
		Forbid: []string{cobra, mcpSDK},
	},
	{
		Name:   "public-core-free-of-adapters",
		Why:    "speaker, voiceconfig and a11y are headed for public API; keep framework and cmd/ code out of them.",
		From:   []string{module + "/internal/speaker", module + "/internal/voiceconfig", module + "/internal/a11y"},
		Forbid: []string{cobra, mcpSDK, module + "/cmd/..."},
	},
	{
		Name:   "only-cmd-imports-cmd",
		Why:    "cmd/ is the adapter layer; shared code belongs in internal/ or pkg/ so core never points up.",
		From:   []string{module + "/..."},
		Except: []string{module + "/cmd/..."},
		Forbid: []string{module + "/cmd/..."},
		Direct: true,
	},
	{
		Name:   "a11y-does-no-io",
		Why:    "internal/a11y is documented as pure functions with no I/O; do the I/O in the caller and pass data in.",
		From:   []string{module + "/internal/a11y"},
		Forbid: []string{"os", "os/exec", "net", "net/http"},
		Direct: true,
	},
	{
		Name:   "pkg-free-of-platform-subsystems",
		Why:    "monitor, recording and clipboard are macOS-specific subsystems; pkg/ must not depend on them.",
		From:   []string{module + "/pkg/..."},
		Forbid: []string{module + "/internal/monitor/...", module + "/internal/recording/...", module + "/internal/clipboard/..."},
	},
}
