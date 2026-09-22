<!-- What this changes and why. Link the issue if there is one. -->

CI does not run on pull requests here (only on version tags), so these are
the checks:

- [ ] `make lint`
- [ ] `make test`
- [ ] `make check-swift-config`, if `VoiceSettings.swift` changed
- [ ] `CHANGELOG.md` has a line under Unreleased, if a user would notice
