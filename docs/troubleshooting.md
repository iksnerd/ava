# Troubleshooting

← [Back to the README](../README.md)

## `no audio recorded` — almost always the microphone permission

This is the most common first-run failure, and the least self-explanatory,
because a denied microphone does not look like an error: `sox` exits
successfully and writes an empty file.

Grant **Microphone** access to whatever you ran the command from — Terminal,
iTerm, Raycast, your editor — under System Settings → Privacy & Security →
Microphone. macOS asks once, per app. If you dismissed that prompt, or the app
already had a stale denial, it will not ask again; add it by hand.

Two things make this easy to misdiagnose:

- The permission belongs to the **parent process**, not to `local-whisper`. The
  same binary works from one terminal and records silence from another.
- Running it over SSH, or from anything without a UI session, cannot prompt at
  all.

Once the permission is right and it still fails, check the input device in
System Settings → Sound.

## `no speech detected` — it heard something, just no words

Different failure: audio was captured, and the transcriber found nothing in it.
Silence detection needs 2 seconds below a 3% threshold, so a noisy room can also
keep the recording going longer than you expect. Speak after the audio cue
rather than over it, and check the input level in System Settings → Sound.

If you passed a file to `local-whisper transcribe` and got this, the file is
readable audio but silent. A file that is not decodable at all reports that
instead, naming what it found in place of a RIFF/WAVE header.

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
local-whisper setup-model               # or: make setup-model, from a checkout
local-whisper setup-model --model tiny  # if you run with --model tiny
```

Downloads `ggml-base.en.bin` (141MB) or `ggml-tiny.en.bin` (74MB) to
`~/.local/share/whisper-cpp/`.

## Transcription feels slow

whisper.cpp spawns a subprocess per run and reloads the model each time, so
every transcription pays that cost — well under a second for a short dictation,
less once the model is in the OS page cache. `--model tiny` trades accuracy for
speed if you need it.

For reference, a 20-second recording transcribes in about 1.3s on an M3 Pro.
If you are seeing much worse, check that `--model` is not pointing at something
unexpected and that the machine is not thermally throttled.

## Speech falls back to the robotic macOS voice

That means the Kokoro server is down and `say` took over. `local-whisper speak`
now says so on stderr when it happens. Start it with:

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
⚠️  voice output is muted — nothing was played; unmute in the Ava menu bar app
```

`local-whisper voices` shows the configured voice, so a wrong-sounding voice is
visible there too.

## The menu bar app and the CLI disagree about a setting

They read the same `~/Library/Application Support/ava/config.json`, and
three separate readers resolve it — bash, Go and Swift. That is guarded by a
contract test (`make test`) plus `make check-swift-config`. If you hit a
disagreement anyway, it's a bug: add the case to
`testdata/voice-config-cases.json` first, then fix whichever reader is wrong.
