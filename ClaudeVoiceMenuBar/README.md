# Claude Voice

A menu bar app for the local voice stack (`../mlx-engine/`, `../scripts/`,
`../` itself): a **Dictate** button for recording/transcribing/pasting at
your cursor (via `local-whisper` — the same dictation tool this whole repo
started as, just triggerable from here instead of only Raycast), a
system-wide **Read Aloud with Claude Voice** right-click action for any
selected text, a global **Mute** switch (also reachable outside the app as
the **Toggle Claude Voice Mute** Service — see below), plus live tuning for
everything Claude Code's Stop/Notification hooks speak through — speed,
volume, voice, spoken-message length, and whether to summarize long
messages with a local LLM instead of cutting them off mid-sentence.

Packaged via `scripts/build-app.sh` into a real `.app` (and a `.dmg`) that
runs from any checkout, not just this one — see "Run it" and "Packaging for
distribution" below.

## Mute

The **Mute Claude Voice** button at the top of the panel is a single global
switch: while it's on, `speak.sh` exits immediately for every caller —
both Claude Code hooks, Read Aloud, and this panel's own Test/Preview
buttons — rather than each one having to remember to check. Muting also
stops anything already playing, and the menu bar icon itself turns into a
muted speaker so you can tell at a glance without opening the panel.

It's also registered as a Service — **Toggle Claude Voice Mute** — with no
selected-text requirement (unlike Read Aloud), so instead of the **Services
→ Text** category below it shows up under System Settings → Keyboard →
Keyboard Shortcuts → Services → **General**, where the same one-time
"new Services start disabled" step applies (turn it on, then optionally
bind a global keyboard shortcut) — but since it needs no selection, that
shortcut works from anywhere, not just apps with classic Services support.
Toggling it that way writes straight to `config.json`; the running app
picks the change up within a second via its own poll (`VoiceSettings.swift`)
rather than only ever trusting its own writes, so the panel and menu bar
icon stay accurate even when muted from outside the app. A system sound
(not speech, deliberately) confirms the toggle when it fires this way.

The small **"Open Keyboard Settings"** link under the Mute button opens
System Settings' Keyboard pane
(`x-apple.systempreferences:com.apple.preference.keyboard`), one click short
of the **Keyboard Shortcuts → Services** screen above. It doesn't jump
straight there: as of macOS 26, none of the documented anchor fragments
(`?Shortcuts`, `?KeyboardShortcuts`, `?Services`) actually navigate anymore
— `log stream` while opening each one shows `System_Settings.OpenBundleArguments
skipReveal:true`, i.e. the anchor goes unrecognized and it just falls back
to the pane's default view ("Modifier Keys"). Rather than ship a link that
confidently lands on the wrong screen, it stops at the Keyboard pane and
leaves the last click to the user.

## Read Aloud with Claude Voice

Select text in any app, right-click (or Edit → Services in the menu bar),
and choose **Services → Read Aloud with Claude Voice** to have it spoken
through the same TTS pipeline as everything else here (current speed,
volume, and voice settings). This only requires the packaged `.app` — no
extra permissions. The menu bar icon fills in while anything is actively
being synthesized/played (see "Speaking indicator" below), whether that's
this, a hook, or the Test/Preview buttons.

It's a standard macOS Service, registered via `NSServices` in the
`.app`'s `Info.plist` (`scripts/build-app.sh`) and handled by
`SpeechService.swift`. Only reaches apps that support the classic
Services mechanism (TextEdit, Notes, Mail, Safari, Pages, Preview, ...) —
Chrome, VS Code, Slack, and most other Electron/Chromium apps never wire
selected text into it, so the item won't appear there no matter what.

**The first time**, macOS adds new Services disabled — open
System Settings → Keyboard → Keyboard Shortcuts → Services → Text, find
"Read Aloud with Claude Voice", and turn it on. That same row is also
where you can bind a global keyboard shortcut to it (select it, click
"Add Shortcut", press the key combo) — the shortcut is still bounded by
the same Services-support limitation above, so it'll fire in TextEdit/
Notes/etc. but not Chrome/VS Code/Slack.

If the item doesn't show up in that list at all yet, macOS's Services
cache can lag a freshly (re)installed app by a minute or so — relaunching
"Claude Voice" (it calls `NSUpdateDynamicServices()` on launch) or logging
out and back in forces a refresh.

There's no length limit worth worrying about — reading a whole article is
fine. `../mlx-engine/server.py`'s `/speak` endpoint stitches Kokoro's
internal ~1200-token chunks back into one file (`join_audio=True`) rather
than returning only the first chunk, and `speak.sh`/`stop-speaking.sh` are
built around exactly this case: a long read taking a while and needing to
be cut short.

## Stop Speaking

Click the menu bar icon while anything is talking and a red **Stop
Speaking** button appears at the top — it cancels whatever's in flight
immediately, whether that's still being synthesized or already playing,
instead of waiting for it to finish. Backed by `scripts/stop-speaking.sh`,
which works off the same per-invocation markers under
`/tmp/claude-tts-active/` that drive the speaking indicator, so it reaches
speech from any source (Read Aloud, a Claude Code hook, Test/Preview) —
including one still queued behind another because only one thing plays at
a time.

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

### Packaging for distribution

`build-app.sh` also bundles `scripts/` (36MB) and a built `local-whisper`
binary (8.3MB) into `Contents/Resources/`, and produces `.build/Claude
Voice.dmg` — so the resulting `.app` runs from *any* checkout, not just
this one. `Paths.swift` resolves `scriptsDir`/`dictateBinary` from that
bundled `Resources/` at runtime when present, falling back to this dev
checkout's paths only when running unbundled (`swift run`).

`mlx-engine/` is deliberately **not** bundled — its venv alone is 1.3GB,
plus a 2.9GB Voxtral model download, not something to freeze into a
distributable app. This has two consequences on a machine that hasn't
separately run `make setup-voxtral`:
- Dictate uses `local-whisper`'s own default `whisper` engine instead of
  Voxtral, so it works with zero extra setup.
- The Server section's Start/Stop button won't find `mlx-engine/`, and
  `speak.sh` already falls back to macOS's built-in `say` when the server's
  unreachable — so the app is fully functional, just with `say`/whisper
  instead of Kokoro/Voxtral's higher quality, until `mlx-engine/` is set up.

**No Apple Developer ID** — the `.dmg`/`.app` are ad-hoc signed
(`codesign -s -`), not notarized. Verified directly (mounted the `.dmg`,
copied the `.app` out, applied a real quarantine attribute the way a
download would, then tried to launch it): opening a quarantined copy of
this exact build is blocked by Gatekeeper's standard "Apple could not
verify ... is free of malware that may harm your Mac" warning — not the
harsher "is damaged and can't be opened" some ad-hoc-signed apps get. The
recipient can either use System Settings → Privacy & Security → scroll
down → "Open Anyway" next to the blocked-app notice (confirm once more in
the follow-up prompt), or run `xattr -cr "/Applications/Claude Voice.app"`
to clear the quarantine attribute directly — also verified: the app
launches clean afterward, no further prompt. Real signing/notarization
needs a paid Apple Developer Program membership — tracked as still-open in
`../TODO.md`.

## What it controls

Every change is written (debounced) to
`~/Library/Application Support/ClaudeVoice/config.json`, which `../scripts/lib.sh`
reads on the shell side — so a setting changed here takes effect on the very
next spoken message, no restart needed. Defaults for any field not yet
present in that file come from `../scripts/voice-defaults.json`.

| Setting | Effect |
|---|---|
| Mute (button) | Global switch — silences hooks, Read Aloud, and Test/Preview until turned off again; also stoppable/settable system-wide via the "Toggle Claude Voice Mute" Service. See "Mute" above. |
| Open Keyboard Settings (link) | Opens System Settings' Keyboard pane; from there, Keyboard Shortcuts → Services is where the two Services below get enabled and can be bound to a global keyboard shortcut — see "Mute" above for why it can't jump straight there |
| Dictate (button) | Runs `local-whisper` (its own default `whisper` engine — works on any Mac, no extra setup) — records, transcribes, copies/pastes at your cursor. Grayed out with an explanatory tooltip if no built binary is found (`make build`/`make install-bin` in the repo root, or the bundled copy in a packaged `.app`). |
| Speed | Kokoro playback speed multiplier |
| Volume | `afplay` output volume |
| Voice | Any of Kokoro's English voices, with a one-click preview |
| "On finish" / "Notification" length | Character cap before a spoken message gets shortened; a "∞" toggle disables the cap entirely for that message type |
| Summarize with local LLM | When a message exceeds its length cap, summarize it with the local Ollama daemon instead of cutting it off mid-sentence (off by default — see `../scripts/hook-stop.sh` and `../scripts/voice_hooks/`) |
| Server status / Start / Stop | Live status of `mlx-engine`, with a manual override — the hooks already auto-start it on demand, this is just visibility |
| Read Aloud with Claude Voice (Services menu) | Speaks the text selected in any app, via the same TTS pipeline and current voice settings — see above |
| Toggle Claude Voice Mute (Services menu / global shortcut) | Flips the same Mute switch from outside the app entirely — see "Mute" above |
| Speaking indicator (menu bar icon) | The waveform icon fills in (`waveform.circle.fill`) while anything is actively being synthesized or played, and reverts once it's done; a muted speaker (`speaker.slash.fill`) takes priority over both states while Mute is on |

## Structure

```
Package.swift                          - bare SPM package, no Xcode project
Sources/ClaudeVoiceMenuBar/
  ClaudeVoiceMenuBarApp.swift           - entry point; hides the Dock icon via
                                          NSApp.setActivationPolicy(.accessory)
  SettingsView.swift                    - the dropdown UI
  Components.swift                      - SettingsRow / SectionCard / StatusBadge
  VoiceSettings.swift                   - config load/save/merge with voice-defaults.json;
                                          polls config.json for external changes (e.g. the
                                          mute Service) and exposes toggleMutedOnDisk()
  ServerController.swift                - polls mlx-engine's /health, drives start/stop
  Speech.swift                          - shells out to scripts/speak.sh
  SpeechService.swift                   - NSServices provider behind "Read Aloud with Claude
                                          Voice" and "Toggle Claude Voice Mute"
  SpeechActivityMonitor.swift           - polls speak.sh's activity marker for the speaking indicator
  Paths.swift                           - resolves scripts/binary from the bundled Resources/ (packaged
                                          build) or this dev checkout (swift run); PATH hardening
Resources/AppIcon.icns
scripts/build-app.sh                    - builds local-whisper + bundles scripts/ into Resources/,
                                          packages + installs the release build, produces a .dmg
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
