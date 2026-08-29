# Claude Voice

A menu bar app for tuning the local voice stack (`../mlx-engine/`, `../scripts/`)
that Claude Code's Stop/Notification hooks speak through — speed, volume,
voice, spoken-message length, and whether to summarize long messages with a
local LLM instead of cutting them off mid-sentence.

A SwiftUI `MenuBarExtra` app, built as a bare Swift Package — there's no
Xcode project, `open Package.swift` (or File → Open in Xcode) gives you a
working scheme directly.

## Run it

```bash
swift run ClaudeVoiceMenuBar          # dev build, runs until you quit or close the terminal
```

For something that survives quitting/reboots, package it as a real `.app`:

```bash
scripts/build-app.sh                  # release build → installs to /Applications/Claude Voice.app
open -a "Claude Voice"
```

To have it launch automatically at login, add a
[LaunchAgent](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
pointing `ProgramArguments` at `open -a "Claude Voice"` with `RunAtLoad`
set, then `launchctl bootstrap gui/$(id -u) <path-to-plist>`. Not included
in this repo since it's a per-machine login item, not project config.

## What it controls

Every change is written (debounced) to
`~/Library/Application Support/ClaudeVoice/config.json`, which `../scripts/lib.sh`
reads on the shell side — so a setting changed here takes effect on the very
next spoken message, no restart needed. Defaults for any field not yet
present in that file come from `../scripts/voice-defaults.json`.

| Setting | Effect |
|---|---|
| Speed | Kokoro playback speed multiplier |
| Volume | `afplay` output volume |
| Voice | Any of Kokoro's English voices, with a one-click preview |
| "On finish" / "Notification" length | Character cap before a spoken message gets shortened; a "∞" toggle disables the cap entirely for that message type |
| Summarize with local LLM | When a message exceeds its length cap, summarize it with the local Ollama daemon instead of cutting it off mid-sentence (off by default — see `../scripts/hook-stop.sh`) |
| Server status / Start / Stop | Live status of `mlx-engine`, with a manual override — the hooks already auto-start it on demand, this is just visibility |

## Structure

```
Package.swift                          - bare SPM package, no Xcode project
Sources/ClaudeVoiceMenuBar/
  ClaudeVoiceMenuBarApp.swift           - entry point; hides the Dock icon via
                                          NSApp.setActivationPolicy(.accessory)
  SettingsView.swift                    - the dropdown UI
  Components.swift                      - SettingsRow / SectionCard / StatusBadge
  VoiceSettings.swift                   - config load/save/merge with voice-defaults.json
  ServerController.swift                - polls mlx-engine's /health, drives start/stop
  Paths.swift                           - where the companion shell scripts live
Resources/AppIcon.icns
scripts/build-app.sh                    - packages + installs the release build
```

`ServerController`'s periodic health poll deliberately backs off while a
Start/Stop action is in flight, then resolves unconditionally once that
action's underlying process actually finishes (`forceRefresh`) — a poll
guard that *also* gated the action's own completion handler would leave the
status permanently stuck on "Starting…"/"Stopping…", since the guard and
the thing meant to clear it would be checking the exact same condition.
