# Troubleshooting

[← Back to the README](../README.md)

## `no audio recorded`: microphone permission

Cause: `sox` exits successfully and writes an empty file when the microphone is
denied, so it does not look like an error. The permission belongs to the parent
process (Terminal, iTerm, Raycast, your editor), not to `ava`, and nothing can
prompt over SSH or without a UI session.

Fix: grant **Microphone** to that app under System Settings → Privacy & Security
→ Microphone. macOS asks only once, so if you dismissed the prompt, add the app
by hand. If it still fails, check the input device in System Settings → Sound.

## `no speech detected`

Cause: audio was captured but the transcriber found no words. Silence detection
needs 2 seconds below a 3% threshold, so a noisy room can keep the recording
going longer than expected.

Fix: speak after the audio cue, and check the input level in System Settings →
Sound. From `ava transcribe FILE`, this message means the file is readable but
silent; an undecodable file reports that instead.

## Nothing pastes

Cause: auto-paste drives Cmd+V through AppleScript, which needs Accessibility
permission for whatever runs the command (Terminal, Raycast, your editor).

Fix: add it under System Settings → Privacy & Security → Accessibility. Without
it the transcript still reaches the clipboard; `ava --no-paste` skips the
attempt.

## `command not found: whisper-cli`

```bash
brew install whisper-cpp
```

## Model not found

```bash
ava setup-model               # or: make setup-model, from a checkout
ava setup-model --model tiny  # if you run with --model tiny
```

Downloads `ggml-base.en.bin` (141MB) or `ggml-tiny.en.bin` (74MB) to
`~/.local/share/whisper-cpp/`.

## Transcription feels slow

Cause: whisper.cpp reloads the model on every run.

Fix: `--model tiny` trades accuracy for speed.

## Speech falls back to the robotic macOS voice

Cause: the Kokoro server is down and `say` took over; `ava speak` says so on
stderr.

Fix:

```bash
ava engine start
ava engine status
```

If it does not come up, read `/tmp/mlx-engine-server.log`. Details are in
[`../mlx-engine/README.md`](../mlx-engine/README.md). Two other stderr notices
mean the same fallback:

- **"the running mlx-engine is older than this ava"**: an engine left running
  across an upgrade. Run `ava engine stop && ava engine start`.
- **"mlx-engine failed to start 3 times in a row; not trying again until 14:05"**:
  three starts failed within five minutes, so `ava` pauses until the oldest
  attempt is five minutes old. Run `ava engine start` to see why it fails; a
  successful start clears the count (the file `ava-engine-start-failures` in
  your temp directory).

## Speech is silent, but nothing errors

Cause: the menu bar app's global Mute is on. Every speech path honours it. The
Claude Code hooks stay quiet, and `ava speak` prints a "voice output is muted"
warning.

Fix: unmute in the menu bar app. `ava voices` shows the configured voice.
