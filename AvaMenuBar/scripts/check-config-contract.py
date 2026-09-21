#!/usr/bin/env python3
"""Run VoiceSettings.swift's config reader against testdata/voice-config-cases.json.

The three readers of ~/Library/Application Support/ava/config.json --
scripts/lib.sh (bash), internal/voiceconfig (Go) and this app (Swift) -- have
to resolve those cases identically. bash and Go are covered automatically by
internal/voiceconfig/contract_test.go, which runs under `make test`. Swift
can't be: the app has no test target, and putting swiftc on the default test
path would break `make test` on a machine without Xcode.

So this is the Swift arm, run on demand (`make check-swift-config`). It
extracts VoiceConfig and its decoder verbatim from VoiceSettings.swift rather
than copying them, so it cannot pass against a stale copy of the type, and it
asserts that load()'s merge still does what the harness replicates.

Exits non-zero on any mismatch. Requires swift on PATH.
"""

import json
import pathlib
import subprocess
import sys
import tempfile

REPO = pathlib.Path(__file__).resolve().parents[2]
SOURCE = REPO / "AvaMenuBar/Sources/AvaMenuBar/VoiceSettings.swift"
FIXTURES = REPO / "testdata/voice-config-cases.json"
DEFAULTS = REPO / "scripts/voice-defaults.json"

# load() is replicated in the harness below; if the real one stops doing these
# things, the harness is testing something the app no longer does.
LOAD_INVARIANTS = (
    "var merged = readJSONObject(defaultsFileURL) ?? [:]",
    "for (key, value) in live { merged[key] = value }",
    "JSONDecoder().decode(VoiceConfig.self, from: data)",
)


def extract_block(source: str, marker: str) -> str:
    """Return `marker` plus its brace-balanced body, verbatim."""
    start = source.index(marker)
    depth, i = 0, source.index("{", start)
    while True:
        if source[i] == "{":
            depth += 1
        elif source[i] == "}":
            depth -= 1
            if depth == 0:
                return source[start : i + 1]
        i += 1


HARNESS = """import Foundation

{struct_src}

{extension_src}

// Replicates VoiceSettings.load()'s merge (see LOAD_INVARIANTS).
func load(defaults: [String: Any], live: [String: Any]?) -> VoiceConfig {{
    var merged = defaults
    if let live = live {{
        for (key, value) in live {{ merged[key] = value }}
    }}
    guard !merged.isEmpty,
          let data = try? JSONSerialization.data(withJSONObject: merged),
          let cfg = try? JSONDecoder().decode(VoiceConfig.self, from: data)
    else {{ return VoiceConfig() }}
    return cfg
}}

func resolved(_ c: VoiceConfig, _ key: String) -> String? {{
    switch key {{
    case "muted": return String(c.muted)
    case "engineAutoStart": return String(c.engineAutoStart)
    case "speed": return String(c.speed)
    case "volume": return String(c.volume)
    case "sayRate": return String(c.sayRate)
    case "voice": return c.voice
    default: return nil
    }}
}}

func same(_ key: String, _ a: String, _ b: String) -> Bool {{
    if key == "speed" || key == "volume" || key == "sayRate" {{
        guard let x = Double(a), let y = Double(b) else {{ return false }}
        return abs(x - y) < 1e-9
    }}
    return a == b
}}

let fixtures = try! JSONSerialization.jsonObject(
    with: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))) as! [String: Any]
let defaults = try! JSONSerialization.jsonObject(
    with: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[2]))) as! [String: Any]

var failures = 0
for raw in fixtures["cases"] as! [[String: Any]] {{
    let name = raw["name"] as! String
    let expect = raw["expect"] as! [String: String]

    var live: [String: Any]? = raw["live"] as? [String: Any]
    if let rawText = raw["raw"] as? String {{
        // An unparseable file reads as absent, same as readJSONObject's nil.
        live = try? JSONSerialization.jsonObject(with: Data(rawText.utf8)) as? [String: Any]
    }} else if raw["live"] is NSNull {{
        live = nil
    }}

    let cfg = load(defaults: defaults, live: live)
    for (key, wantRaw) in expect {{
        guard let got = resolved(cfg, key) else {{ continue }}
        var want = wantRaw
        if want == "default" {{
            // `d is Bool` is true for every NSNumber JSONSerialization makes,
            // so key off the known kinds rather than the ObjC bridge.
            switch key {{
            case "muted", "engineAutoStart":
                want = String((defaults[key]! as! NSNumber).boolValue)
            case "speed", "volume", "sayRate":
                want = String((defaults[key]! as! NSNumber).doubleValue)
            default:
                want = defaults[key]! as! String
            }}
        }}
        if !same(key, got, want) {{
            print("FAIL  \\(name): \\(key) = \\(got), want \\(want)")
            failures += 1
        }}
    }}
}}
print(failures == 0 ? "PASS  Swift matches the config contract" : "FAILURES: \\(failures)")
exit(failures == 0 ? 0 : 1)
"""


def main() -> int:
    source = SOURCE.read_text()
    for invariant in LOAD_INVARIANTS:
        if invariant not in source:
            print(f"VoiceSettings.load() no longer contains: {invariant}", file=sys.stderr)
            print("The harness replicates load(); update both together.", file=sys.stderr)
            return 2

    harness = HARNESS.format(
        struct_src=extract_block(source, "struct VoiceConfig: Codable, Equatable {"),
        extension_src=extract_block(source, "extension VoiceConfig {"),
    )

    with tempfile.TemporaryDirectory() as tmp:
        path = pathlib.Path(tmp) / "contract.swift"
        path.write_text(harness)
        # No pipe: the exit status has to be swift's, not a pager's.
        return subprocess.run(["swift", str(path), str(FIXTURES), str(DEFAULTS)]).returncode


if __name__ == "__main__":
    sys.exit(main())
