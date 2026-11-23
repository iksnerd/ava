# Ava agent guide

Ava is a local voice stack for macOS: the `ava` Go CLI (dictation, speech, transcription, an MCP server, `ava monitor`) plus the `mlx-engine`, `voxtral`, `scripts` and `AvaMenuBar` components.

[`CLAUDE.md`](CLAUDE.md) is the project guide for Go work: layout, conventions, ports. [`docs/`](docs/) holds the per-component docs, starting with [`docs/cli.md`](docs/cli.md) and [`docs/architecture.md`](docs/architecture.md).

```bash
make build   # binary to bin/ava
make test    # Go, voice hooks, mlx-engine, voxtral
make lint    # run before committing
```
