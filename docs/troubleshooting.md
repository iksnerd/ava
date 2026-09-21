# Troubleshooting

← [Back to the README](../README.md)

## Recording never stops, or no speech is detected

Silence detection needs 2 seconds below a 3% threshold, so a noisy room can
keep it recording. Check the input device in System Settings → Sound, and speak
after the audio cue, not over it.

## Nothing pastes

Auto-paste drives Cmd+V through AppleScript, which needs Accessibility
permission for whatever runs the command — Terminal, Raycast, or your editor.
Add it under System Settings → Privacy & Security → Accessibility.

The transcript still reaches the clipboard without that permission. Use
`local-whisper --no-paste` to skip the attempt entirely.

## `command not found: whisper-cli`

```bash
brew install whisper-cpp
```

## Model not found

```bash
make setup-model
```

Downloads `ggml-base.en.bin` (141MB) to `~/.local/share/whisper-cpp/`.

## Transcription feels slow

The `whisper` engine spawns a subprocess per run and reloads the model each
time, so every transcription pays that cost — a few seconds, less once the file
is in the OS page cache. `--model tiny` trades accuracy for speed.

`--engine voxtral` keeps a warm server instead, which is faster after the first
request. It needs `make setup-voxtral` first.

## `voxtral server is not running or model failed to load`

```bash
local-whisper engine start
local-whisper engine status
```

If it doesn't come up, the server's log is at `/tmp/mlx-engine-server.log`.
Full detail in [`../mlx-engine/README.md`](../mlx-engine/README.md).

## Speech is silent, but nothing errors

Check the menu bar app's global Mute. Every speech path honours it, and
`local-whisper speak` prints:

```
⚠️  voice output is muted — nothing was played; unmute in the Claude Voice menu bar app
```

`local-whisper voices` shows the configured voice, so a wrong-sounding voice is
visible there too.

## The menu bar app and the CLI disagree about a setting

They read the same `~/Library/Application Support/ClaudeVoice/config.json`, and
three separate readers resolve it — bash, Go and Swift. That is guarded by a
contract test (`make test`) plus `make check-swift-config`. If you hit a
disagreement anyway, it's a bug: add the case to
`testdata/voice-config-cases.json` first, then fix whichever reader is wrong.
