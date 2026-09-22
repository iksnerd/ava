import AppKit
import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var settings: VoiceSettings
    @EnvironmentObject private var activity: SpeechActivityMonitor
    @StateObject private var server = ServerController()
    @StateObject private var ollama = OllamaStatus()

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            header
            errorBanner

            Button {
                toggleMute()
            } label: {
                Label(
                    settings.config.muted ? "Unmute Ava" : "Mute Ava",
                    systemImage: settings.config.muted ? "speaker.slash.fill" : "speaker.wave.2.fill"
                )
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(settings.config.muted ? .red : .indigo)
            .help(
                settings.config.muted
                    ? "Hooks, Read Aloud, and Test/Preview are all silenced — this is the same switch as the \"Toggle Ava Mute\" Service"
                    : "Silences Claude Code hooks, Read Aloud, and Test/Preview — also available system-wide as the \"Toggle Ava Mute\" Service"
            )

            Button {
                dictate()
            } label: {
                Label("Dictate", systemImage: "mic.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(.indigo)
            .disabled(VoicePaths.dictateBinary == nil)
            .help(
                VoicePaths.dictateBinary != nil
                    ? "Record, transcribe, and paste at your cursor"
                    : "ava binary not found — run `make build` (or `make install-bin`) in the repo"
            )

            SectionCard(header: "Playback") {
                SettingsRow(
                    icon: "hare.fill", tint: .orange, title: "Speed",
                    subtitle: String(format: "%.2fx", settings.config.speed)
                ) {
                    Slider(value: $settings.config.speed, in: 0.5...2.5) { Text("Speed") }
                        .labelsHidden()
                        .frame(width: 116)
                        .accessibilityLabel("Speed")
                        .accessibilityValue(String(format: "%.2f times", settings.config.speed))
                }
                SettingsRow(
                    icon: "speaker.wave.2.fill", tint: .teal, title: "Volume",
                    subtitle: String(format: "%.0f%%", settings.config.volume * 100)
                ) {
                    Slider(value: $settings.config.volume, in: 0...1) { Text("Volume") }
                        .labelsHidden()
                        .frame(width: 116)
                        .accessibilityLabel("Volume")
                        .accessibilityValue(String(format: "%.0f percent", settings.config.volume * 100))
                }
                if settings.config.volume == 0 && !settings.config.muted {
                    Text("Volume is at zero, so nothing will be audible even though Mute is off.")
                        .font(.system(size: 9.5))
                        .foregroundStyle(.orange)
                        .fixedSize(horizontal: false, vertical: true)
                }
                SettingsRow(icon: "person.wave.2.fill", tint: .purple, title: "Voice", subtitle: settings.config.voice) {
                    HStack(spacing: 6) {
                        Picker("", selection: $settings.config.voice) {
                            ForEach(KokoroVoice.english, id: \.self) { voice in
                                Text(voice).tag(voice)
                            }
                        }
                        .labelsHidden()
                        .frame(width: 82)

                        Button {
                            previewVoice()
                        } label: {
                            Image(systemName: "play.fill")
                                .accessibilityHidden(true)
                        }
                        .accessibilityLabel("Preview \(settings.config.voice) voice")
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                        .disabled(settings.config.muted)
                        .help(settings.config.muted ? "Unmute to preview a voice" : "Preview \(settings.config.voice)")
                    }
                }
            }

            SectionCard(header: "Spoken messages") {
                limitedLengthRow(
                    icon: "checkmark.message.fill", tint: .green, title: "On finish",
                    keyPath: \.stopMaxChars, range: 100...1200, defaultValue: 600
                )
                .help(
                    settings.config.llmSummary
                        ? "Target length for the LLM summary when Claude finishes responding — a soft budget given to the model, not a hard cutoff"
                        : "How long the spoken summary can be when Claude finishes responding, before it's cut off mid-sentence"
                )
                limitedLengthRow(
                    icon: "bell.badge.fill", tint: .red, title: "Notification",
                    keyPath: \.notifyMaxChars, range: 100...1200, defaultValue: 500
                )
                .help("How long a spoken notification (permission prompts, waiting for input) can be, before it's cut off mid-sentence — always a hard cutoff, LLM summary doesn't apply here")
                SettingsRow(icon: "sparkles", tint: .pink, title: "LLM summary", subtitle: settings.config.summaryModel) {
                    Toggle("", isOn: $settings.config.llmSummary).labelsHidden()
                }
                .help("Summarize long \"On finish\" messages with the local Ollama model instead of a mid-sentence cutoff. Only applies to \"On finish\" — notifications are always truncated.")

                if settings.config.llmSummary {
                    Text("Only applies to \"On finish\" — notifications are always truncated, never summarized.")
                        .font(.system(size: 9.5))
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    // Turning this on used to be unverifiable: with Ollama down
                    // or the model unpulled, hooks quietly truncate instead and
                    // nothing says why.
                    if let problem = ollama.state.problem {
                        Text(problem)
                            .font(.system(size: 9.5))
                            .foregroundStyle(.orange)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }

            SectionCard(header: "Server") {
                SettingsRow(icon: "server.rack", tint: statusColor, title: "mlx-engine") {
                    HStack(spacing: 8) {
                        StatusBadge(text: statusText, color: statusColor)
                        Button(server.status == .running ? "Stop" : "Start") {
                            server.status == .running ? server.stop() : server.start()
                        }
                        .controlSize(.small)
                        .disabled(server.status == .starting || server.status == .stopping)
                        .help(
                            server.status == .running
                                ? "Stopping also keeps Claude Code hooks from silently restarting it, until you press Start again"
                                : "Start again to let Claude Code hooks auto-start the server on demand"
                        )
                    }
                }
                if server.status == .stopped && !settings.config.engineAutoStart {
                    Text("Hooks won't auto-restart the server until you press Start.")
                        .font(.system(size: 9.5))
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            VStack(spacing: 8) {
                if activity.isSpeaking {
                    Button(role: .destructive) {
                        Speech.stop()
                    } label: {
                        Label("Stop Speaking", systemImage: "stop.fill")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.red)
                    .controlSize(.regular)
                }

                Button {
                    runTest()
                } label: {
                    Label("Speak a Test Phrase", systemImage: "waveform")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.regular)
                .disabled(settings.config.muted)
                .help(settings.config.muted ? "Unmute to hear a test phrase" : "")

                // A once-ever setup link; it used to sit between Mute and
                // Dictate, splitting the two primary actions.
                Button {
                    openServicesSettings()
                } label: {
                    Label("Open Keyboard Settings", systemImage: "arrow.up.forward.square")
                        .font(.system(size: 10))
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
                .help(
                    "From there, open Keyboard Shortcuts → Services to enable \"Toggle Ava Mute\" and \"Read Aloud with Ava\" and optionally bind each to a global keyboard shortcut — macOS doesn't currently support deep-linking straight to that screen"
                )

                Button("Quit") { NSApp.terminate(nil) }
                    .buttonStyle(.plain)
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
            }
        }
        .padding(14)
        .frame(width: 312)
        .onAppear { ollama.track(enabled: settings.config.llmSummary, model: settings.config.summaryModel) }
        .onChange(of: settings.config.llmSummary) { on in
            ollama.track(enabled: on, model: settings.config.summaryModel)
        }
        .onChange(of: settings.config.summaryModel) { model in
            ollama.track(enabled: settings.config.llmSummary, model: model)
        }
    }

    /// The panel had no error surface at all: nothing anywhere said "the last
    /// thing you asked me to do failed". Shown under the header so it is the
    /// first thing read, and only when there is something to say.
    @ViewBuilder
    private var errorBanner: some View {
        if let message = server.lastError ?? Speech.lastError {
            HStack(alignment: .top, spacing: 6) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.system(size: 10))
                    .foregroundStyle(.orange)
                Text(message)
                    .font(.system(size: 10.5))
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .padding(8)
            .background(Color.orange.opacity(0.1), in: RoundedRectangle(cornerRadius: 6, style: .continuous))
            .accessibilityLabel("Error: \(message)")
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            Image(systemName: "waveform")
                .font(.system(size: 13, weight: .bold))
                .foregroundStyle(.white)
                .frame(width: 26, height: 26)
                .background(Color.indigo.gradient, in: RoundedRectangle(cornerRadius: 7, style: .continuous))
            VStack(alignment: .leading, spacing: 0) {
                Text("Ava").font(.system(size: 14, weight: .semibold))
                Text("Dictation + spoken Claude Code updates")
                    .font(.system(size: 10.5))
                    .foregroundStyle(.secondary)
            }
            Spacer()
        }
    }

    private var statusColor: Color {
        switch server.status {
        case .running: return .green
        case .stopped: return .gray
        case .starting, .stopping: return .orange
        case .unknown: return .gray
        }
    }

    private var statusText: String {
        switch server.status {
        case .running: return "Running"
        case .stopped: return "Stopped"
        case .starting: return "Starting…"
        case .stopping: return "Stopping…"
        case .unknown: return "Checking…"
        }
    }

    // Stored as <= 0 in config to mean "no limit" (read by hook-stop.sh /
    // hook-notify.sh as unlimited). The slider shows/edits `defaultValue`
    // while unlimited so it lands somewhere sane if the toggle is turned back off.
    private func limitedLengthRow(
        icon: String, tint: Color, title: String, keyPath: WritableKeyPath<VoiceConfig, Int>,
        range: ClosedRange<Double>, defaultValue: Int
    ) -> some View {
        let current = settings.config[keyPath: keyPath]
        let isUnlimited = current <= 0

        let valueBinding = Binding<Double>(
            get: { let v = settings.config[keyPath: keyPath]; return Double(v > 0 ? v : defaultValue) },
            set: { settings.config[keyPath: keyPath] = Int($0) }
        )
        let unlimitedBinding = Binding<Bool>(
            get: { settings.config[keyPath: keyPath] <= 0 },
            set: { on in settings.config[keyPath: keyPath] = on ? -1 : defaultValue }
        )

        return SettingsRow(
            icon: icon, tint: tint, title: title,
            subtitle: isUnlimited ? "No limit" : "\(current) characters"
        ) {
            HStack(spacing: 6) {
                Slider(value: valueBinding, in: range) { Text(title) }
                    .labelsHidden()
                    .frame(width: 78)
                    .disabled(isUnlimited)
                    // A disabled slider still rendered at its stored position,
                    // so the panel displayed a number that was not in effect.
                    .opacity(isUnlimited ? 0.35 : 1)
                    .accessibilityLabel("\(title) length limit")
                    .accessibilityValue(isUnlimited ? "no limit" : "\(current) characters")
                Toggle("∞", isOn: unlimitedBinding)
                    .toggleStyle(.button)
                    .help(isUnlimited
                        ? "Currently unlimited: \(title.lowercased()) messages are spoken in full. Click to cap the length again."
                        : "Click for no length limit — \(title.lowercased()) messages get spoken in full instead of capped.")
                    .accessibilityLabel("No length limit for \(title.lowercased())")
                    .controlSize(.mini)
            }
        }
    }

    // Mirrors the "Toggle Ava Mute" Service (SpeechService.swift) —
    // both flip the same config.json field, so muting from either place
    // stays in sync (the Service's own writes are picked up here via
    // VoiceSettings' external-change poll).
    private func toggleMute() {
        settings.config.muted.toggle()
        if settings.config.muted { Speech.stop() }
    }

    // No anchor fragment (?Shortcuts, ?KeyboardShortcuts, ?Services) reliably
    // lands on the Keyboard Shortcuts screen as of macOS 26 — all three were
    // tried and each fell back to the pane's default view ("Modifier Keys"),
    // confirmed via `log stream` showing `OpenBundleArguments skipReveal:true`
    // for the request (i.e. the anchor wasn't recognized, so it didn't
    // navigate anywhere in particular). Rather than ship a link that
    // confidently lands on the wrong screen, this just opens the Keyboard
    // pane itself — the tooltip above tells the user the one extra click.
    private func openServicesSettings() {
        guard let url = URL(string: "x-apple.systempreferences:com.apple.preference.keyboard") else {
            return
        }
        NSWorkspace.shared.open(url)
    }

    private func runTest() {
        settings.saveNow()
        Speech.speak("This is Ava, speaking at the current speed, volume, and voice settings.")
    }

    private func previewVoice() {
        settings.saveNow()
        Speech.speak("This is the \(settings.config.voice) voice.", voice: settings.config.voice)
    }

    private func dictate() {
        guard let bin = VoicePaths.dictateBinary else {
            Speech.lastError = "Could not find the ava binary. Reinstall the app."
            return
        }
        let task = Process()
        task.executableURL = URL(fileURLWithPath: bin)
        // No -engine flag: ava's own default (whisper) works on
        // any Mac with zero extra setup, unlike voxtral (Apple Silicon +
        // mlx-engine's 1.3GB venv + a 2.9GB model download).
        task.environment = VoicePaths.hardenedEnvironment
        do {
            try task.run()
            Speech.lastError = nil
        } catch {
            // Was `try?`. Dictation gives no other feedback — the panel closes
            // and recording begins invisibly — so a failure to launch was
            // indistinguishable from a working mic that heard nothing.
            Speech.lastError = "Could not start dictation: \(error.localizedDescription)"
        }
    }
}
