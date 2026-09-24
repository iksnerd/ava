# Tuning the engines

What each engine actually exposes, what it does not, and which knobs are worth
reaching for. Everything here was checked against the installed versions rather
than copied from upstream docs.

← [Back to the README](../README.md)

## Kokoro (text-to-speech)

### The whole knob surface

`mlx_audio`'s `generate_audio()` advertises about 25 parameters — `temperature`,
`cfg_scale`, `ddpm_steps`, `sigma`, `ref_audio`, `stream`, `use_zero_spk_emb`
and so on. **Almost none of them are Kokoro's.** They exist because that function
is shared across every TTS model `mlx_audio` supports, and the ones that do not
apply are dropped.

Kokoro's own `generate()` accepts exactly five things:

| Parameter | Exposed as | Notes |
| --- | --- | --- |
| `text` | `text` | Required |
| `voice` | `voice` | A voice id, or several comma-separated to blend |
| `speed` | `speed` | Rate multiplier; 1.0 is natural |
| `lang_code` | *derived* | Taken from the voice id's first letter — see below |
| `split_pattern` | `split_pattern` | Chunking regex. Defaults to blank lines |

So the API surface is not missing much. `/speak` takes `text`, `voice` and
`speed`, and derives `lang_code`. **Anything else is rejected with a 422**
rather than accepted and ignored, which is what used to happen — a request
carrying `temperature` returned 200 and changed nothing.

### The voice id carries the accent rules

The first letter of the voice id selects the phonemizer locale: `a` American
English, `b` British, `e` Spanish, and so on. Without it every voice is
phonemized with American rules, so a British voice speaks in the right timbre
with the wrong pronunciation — audible only if you are listening for it. The
server derives this from the voice you pass, so it is automatic.

### Blending is an average, and it is not limited to two

`KokoroPipeline.load_voice` splits on commas and returns `mx.mean` over the
stacked style vectors, so a blend is a plain arithmetic average and **any number
of voices works**:

```bash
ava speak --voice af_heart,bf_emma "a fifty-fifty blend"
ava speak --voice af_heart,bf_emma,am_adam "three-way average"
```

Verified against the running server: three ids return 200 and produce audio.
Each id must be known — an unknown one is an error now, not a silent fallback
to the macOS voice. `ava voices` lists all 28.

### Intonation and stress

This is the real prosody control, and it lives in the text rather than in a
parameter. `misaki`, the phonemizer, reads a markdown-link syntax —
`[word](payload)` — and interprets the payload four ways:

| Payload | Effect |
| --- | --- |
| `/ɹɪpˈoːt/` | Replace the word's phonemes outright, in IPA |
| an integer, e.g. `2`, `-1` | Shift stress |
| `0.5`, `+0.5`, `-0.5` | Half-step stress shift |
| `#…#` | Number-pronunciation flags, not prosody |

Anything else in the parens is ignored silently.

The stress scale is easiest to read as phonemes. Running `misaki` directly on
the same sentence, where `ˈ` is primary stress and `ˌ` is secondary:

```
plain          ðə ɹəpˈɔɹt ɪz dˈu tədˈA.
[report](2)    ðə ɹəpˈɔɹt ɪz dˈu tədˈA.     unchanged — it already had primary
[report](-1)   ðə ɹəpˌɔɹt ɪz dˈu tədˈA.     primary demoted to secondary
[report](-2)   ðə ɹəpɔɹt  ɪz dˈu tədˈA.     stress removed entirely
[report](/ɹɪpˈoːt/)
               ðə ɹɪpˈoːt ɪz dˈu tədˈA.     phonemes replaced
```

So in practice: **negative values de-emphasise**, and positive values only do
something when the word was not already stressed — raising a word that already
carries primary stress is a no-op. Adding stress to an unstressed word gives it
secondary stress.

Explicit IPA is the tool for names, acronyms and jargon it mangles, and it is
more reliable than any amount of stress nudging.

### What is not exposed, and why

Underneath, `Model.__call__` takes a `ref_s` style vector — the StyleTTS2
mechanism that actually carries prosody. It is not reachable through the API,
and there is a wrinkle worth knowing even so: the voice file is a *pack* of
style vectors, and the pipeline picks one by phoneme count
(`pipeline.py`: `pack[len(ps) - 1]`). **The same voice therefore has slightly
different prosody for a short utterance than a long one.** That is by design,
not drift.

`split_pattern` is exposed on `/speak`. Kokoro's default splits on blank lines,
which is the wrong unit for text that arrives as one long paragraph — that lands
as a single chunk and runs into the token cap. Pass `"\\. "` to chunk by
sentence instead. Omit it entirely to keep Kokoro's default; sending `null` is
the same as omitting it.

### Output is not deterministic

Identical requests produce different audio. Measured over three byte-identical
`/speak` calls: same length every time (88,844 bytes), but **37.9% of the PCM
samples differed** between runs. The variation is inaudible in practice.

Two consequences worth knowing:

- Do not cache or compare by hashing the audio. Hash the request instead.
- A/B-ing two voices means listening, not diffing.

### Long text

Kokoro splits text past roughly 1,200 tokens into segments and the server
stitches them with `join_audio`, so arbitrarily long input comes back as one
file rather than silently ending after a paragraph. A 300-word passage
synthesises in about 1.3s and returns ~20s of audio.

### Speed

Warm, on an M3 Pro, roughly **16x faster than real time** — about 0.13s for a
2-second utterance. The first call after the server starts costs ~2.6s more,
which is MLX compiling; the server pays that at startup on purpose so no real
request does.

## whisper.cpp (speech-to-text)

### What the wrapper passes

`pkg/stt/whisper` runs `whisper-cli` with:

| Flag | Value | Why |
| --- | --- | --- |
| `-m` | the model path | `base.en` by default, `tiny.en` with `--model tiny` |
| `-f` | the audio file | 16kHz mono WAV, which `internal/audio` guarantees |
| `-t` | `8` | Threads. The binary defaults to 4 |
| `-nt` | — | No timestamps, so stdout *is* the transcript |
| `-sns` | — | Suppress non-speech tokens `(coughs)`, `[music]` |
| `-l` | `--lang` | Language code |
| `--prompt` | the context file | Vocabulary hints — see below |
| `-bs` | `--beam-size` | Only when set; otherwise whisper-cli's default of 5 applies |

`--beam-size` is on dictation, `transcribe` and the MCP `transcribe` tool
(`beam_size`). Lowering it trades accuracy for speed, which is mostly worth
doing on `tiny`: `--model tiny --beam-size 1` is greedy decoding on the
smallest model.

### Vocabulary hints do more than any flag

A `.whisper-context` file is the highest-leverage tuning available, and it
needs no flags:

```bash
echo "Kubernetes, kubectl, Istio, Prometheus, Grafana" > .whisper-context
```

Per-directory beats `~/.whisper-context`; `--context` overrides both. This is
how you stop it writing "cube control". See
[`cli.md`](cli.md#context-files).

### Flags the wrapper does not expose

`whisper-cli` offers plenty more. None of these is wired up, and this is the
honest reason: none has been needed. If you want one, follow `--beam-size`:
a field on `stt.Options`, appended in `pkg/stt/whisper`'s `buildArgs` only when
set, and a flag on dictation, `transcribe` and the MCP tool.

| Flag | Default | Might matter when |
| --- | --- | --- |
| `-bo`, `--best-of` | 5 | Same trade |
| `-tp`, `--temperature` | 0.0 | Greedy by default; raising it rarely helps dictation |
| `-nth`, `--no-speech-thold` | 0.60 | Recordings it wrongly calls silent |
| `-et`, `--entropy-thold` | 2.40 | Decoder giving up mid-segment |
| `-mc`, `--max-context` | -1 | Constraining cross-segment context drift |
| `-tr`, `--translate` | off | Translating to English instead of transcribing |
| `-di`, `--diarize` | off | Stereo speaker separation — different from `ava monitor --diarize` |
| `-p`, `--processors` | 1 | Long files on many cores |

Run `whisper-cli --help` for the full list.

### Speed

`base.en` transcribes a 20-second recording in about **1.26s** on an M3 Pro,
roughly 16x faster than real time. `tiny.en` is faster and noticeably less
accurate. The model reloads on every run, since each transcription is its own
subprocess — that cost is already in the number above.

## Recording

Set in `internal/audio` and `internal/recording`, not configurable at runtime:

- **16kHz mono**, which is what whisper.cpp wants
- **`norm -3`** peak normalisation, applied in the same `sox` call as capture
- **2 seconds below a 3% threshold** ends the recording
- **`--highpass-hz`** on `ava monitor` only. The flag defaults to `0`, which
  means "use the built-in default" of 80Hz — `0` does not disable it. It matters
  more for loopback capture than for a microphone, since loopback audio runs
  much quieter so the noise floor weighs more. See
  [`monitor.md`](monitor.md)
