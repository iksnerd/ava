# Ava menu bar app

A macOS menu bar app (SwiftUI `MenuBarExtra`) for Ava's dictation and voice stack.

## What it does

- **Dictate**: runs `ava` to record, transcribe with whisper.cpp and paste at the cursor. Disabled when no `ava` binary is found.
- **Read Aloud with Ava**: a macOS Service that speaks the selected text in any app that supports Services (TextEdit, Notes, Mail, Safari; not Chrome or Electron apps).
- **Mute**: one global switch that silences the Claude Code hooks, Read Aloud and the panel's Test and Preview buttons. The **Toggle Ava Mute** Service flips it from any app or a global shortcut.
- **Stop Speaking**: appears while anything is speaking and cancels it, queued speech included.
- **Voice tuning**: speed, volume, voice, spoken-message length, and optional summarizing of long messages with a local Ollama model. Changes apply to the next spoken message.
- **Engine**: shows the Kokoro server's status and has Start and Stop buttons.
- **Set up Ava**: shown only when `ava setup --check` finds something missing. It runs `ava setup` and reports progress. Without Homebrew it links to brew.sh.

The menu bar icon shows the state: a muted speaker while muted, a filled waveform while speaking, a plain waveform otherwise.

## Get it

Download `Ava.dmg` from the [latest release](https://github.com/iksnerd/ava/releases/latest) and follow [the menu bar app guide](../docs/menu-bar-app.md).

## Build and run from source

Needs the Xcode Command Line Tools (`swift`).

```bash
swift run AvaMenuBar        # dev build; needs a repo checkout and `make build` for Dictate
scripts/build-app.sh        # release build, installs /Applications/Ava.app, writes .build/Ava.dmg
```

The app has no launch-at-login setting. Add `/Applications/Ava.app` under System Settings, General, Login Items.

`make release-dmg` builds the `.dmg` from a clean, tagged checkout and attaches it to that GitHub release with an updated `checksums.txt`.

## How packaging works

`build-app.sh` builds the release binary and `ava`, then assembles `Ava.app`. It writes `Info.plist` itself, including the two `NSServices` entries.

| In the bundle | Purpose |
|---|---|
| `Resources/scripts/` (without `.venv` and `__pycache__`) | the shell side: `speak.sh`, `stop-speaking.sh`, `mlx-engine-server.sh` |
| `Resources/ava` | the Dictate binary and `ava setup` |
| `Resources/engine-root` | the checkout the app was built from (`.app` only) |

`Paths.swift` uses the bundled `Resources/` when it exists. Without it (`swift run`) it falls back to the checkout's `scripts/`, then `bin/ava`, then `~/.local/bin/ava`.

`mlx-engine/` is not bundled; its virtualenv is too large. The bundled `mlx-engine-server.sh` looks for an engine in this order, and Start and speech auto-start use the first one found:

1. `mlx-engine/` inside the bundle (not shipped today).
2. The checkout recorded in `Resources/engine-root`.
3. The engine installed by `ava setup` (`~/Library/Application Support/ava/engine`, or `$AVA_ENGINE_DIR`).

With no engine, Start reports it and speech falls back to macOS `say`. Dictate does not need the engine. If the checkout moves, rebuild the app so `engine-root` is correct.

`engine-root` is removed from the `.dmg`, because a path from the build machine is wrong on another Mac and would leak a home directory.

### Signing

The app and `.dmg` are ad-hoc signed (`codesign -s -`), not notarized, because there is no Apple Developer ID. Gatekeeper blocks a downloaded copy. Either:

- System Settings, Privacy & Security, scroll down, **Open Anyway**, then confirm; or
- `xattr -cr /Applications/Ava.app`.

## Services

macOS adds new Services disabled. Turn them on under System Settings, Keyboard, Keyboard Shortcuts, Services: **Read Aloud with Ava** is under Text, **Toggle Ava Mute** under General. This is also where you bind a shortcut. The panel's **Open Keyboard Settings** link opens the Keyboard pane; macOS does not allow a link straight to the Services list.

A freshly installed app can take a minute to appear in other apps' Services menus. Relaunching Ava (it calls `NSUpdateDynamicServices()` at launch) or logging out and back in refreshes it.

## Settings storage

Changes are written, debounced, to `~/Library/Application Support/ava/config.json`. The shell scripts and `ava` read the same file, and missing fields take the defaults in `scripts/voice-defaults.json`. The app polls the file once a second, so a mute toggled from outside (the Service, `ava`) shows up in the panel.

## Code map

`Sources/AvaMenuBar/`:

```
AvaMenuBarApp.swift          entry point; hides the Dock icon; menu bar icon state
SettingsView.swift           the panel UI
Components.swift             SettingsRow, SectionCard, StatusBadge
VoiceSettings.swift          load, save and merge config.json; polls for outside changes
ServerController.swift       polls the engine's /health; Start and Stop
Speech.swift                 runs scripts/speak.sh and stop-speaking.sh
SpeechService.swift          NSServices handlers: Read Aloud, Toggle Mute
SpeechActivityMonitor.swift  polls the activity marker behind the speaking indicator
SetupController.swift        runs `ava setup --check` and `ava setup`
OllamaStatus.swift           checks Ollama and the configured model for summarizing
Paths.swift                  script and binary lookup; PATH hardening
Protocol.swift               generated from internal/protocol/protocol.json; do not edit
```

Other files: `Resources/AppIcon.icns`, `scripts/build-app.sh`, `scripts/check-config-contract.py`.

A GUI app launched by launchd inherits only `/usr/bin:/bin:/usr/sbin:/sbin`, so `sox` in Homebrew's directory is not on its PATH. `VoicePaths.hardenedEnvironment` prepends `/opt/homebrew/bin`, `/usr/local/bin` and `~/.local/bin` for every process the app spawns.

## Testing

There are no Swift unit tests. CI runs `swift build --package-path AvaMenuBar` and `make check-swift-config`, which checks that `VoiceSettings.swift` resolves the config the same way as the bash and Go readers. `make check-swift-config` needs `swift` and is not part of `make lint`.
