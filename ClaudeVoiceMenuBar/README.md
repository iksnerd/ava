# Claude Voice

A menu bar app for the local voice stack (`../mlx-engine/`, `../scripts/`,
`../` itself): a **Dictate** button for recording/transcribing/pasting at
your cursor (via `local-whisper -engine voxtral` — the same dictation tool
this whole repo started as, just triggerable from here instead of only
Raycast), plus live tuning for everything Claude Code's Stop/Notification
hooks speak through — speed, volume, voice, spoken-message length, and
whether to summarize long messages with a local LLM instead of cutting them
off mid-sentence.

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
| Dictate (button) | Runs `local-whisper -engine voxtral` — records, transcribes, copies/pastes at your cursor. Grayed out with an explanatory tooltip if no built binary is found (`make build` or `make install-bin` in the repo root). |
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
  Paths.swift                           - where the companion shell scripts/binary live, PATH hardening
Resources/AppIcon.icns
scripts/build-app.sh                    - packages + installs the release build
```

`ServerController`'s periodic health poll deliberately backs off while a
Start/Stop action is in flight, then resolves unconditionally once that
action's underlying process actually finishes (`forceRefresh`) — a poll
guard that *also* gated the action's own completion handler would leave the
status permanently stuck on "Starting…"/"Stopping…", since the guard and
the thing meant to clear it would be checking the exact same condition.

A GUI app launched via LaunchServices/launchd (as this one is, once packaged
— see the LaunchAgent note above) inherits a minimal PATH: just
`/usr/bin:/bin:/usr/sbin:/sbin`, not Homebrew's `/opt/homebrew/bin` where
`sox` lives. `Dictate` shells out to the `local-whisper` binary directly (no
wrapping script to fix this the way `speak.sh`/`voxtral-server.sh` do via
`lib.sh`), so it failed silently — the binary launched, its own dependency
check couldn't find `sox` on `PATH`, and it exited immediately, with nothing
surfaced since `Process.run()`'s error wasn't being read. `VoicePaths.hardenedEnvironment`
fixes this by setting `PATH` explicitly on every `Process` this app spawns.
