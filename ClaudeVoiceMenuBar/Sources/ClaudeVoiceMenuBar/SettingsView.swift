import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var settings: VoiceSettings
    @StateObject private var server = ServerController()

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            header

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
                    : "local-whisper binary not found — run `make build` (or `make install-bin`) in the repo"
            )

            SectionCard(header: "Playback") {
                SettingsRow(
                    icon: "hare.fill", tint: .orange, title: "Speed",
                    subtitle: String(format: "%.2fx", settings.config.speed)
                ) {
                    Slider(value: $settings.config.speed, in: 0.5...2.5).frame(width: 116)
                }
                SettingsRow(
                    icon: "speaker.wave.2.fill", tint: .teal, title: "Volume",
                    subtitle: String(format: "%.0f%%", settings.config.volume * 100)
                ) {
                    Slider(value: $settings.config.volume, in: 0...1).frame(width: 116)
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
                    }
                }
            }

            SectionCard(header: "Spoken messages") {
                limitedLengthRow(
                    icon: "checkmark.message.fill", tint: .green, title: "On finish",
                    keyPath: \.stopMaxChars, range: 100...1200, defaultValue: 600
                )
                .help("How long the spoken summary can be when Claude finishes responding")
                limitedLengthRow(
                    icon: "bell.badge.fill", tint: .red, title: "Notification",
                    keyPath: \.notifyMaxChars, range: 100...1200, defaultValue: 500
                )
                .help("How long a spoken notification (permission prompts, waiting for input) can be")
                SettingsRow(icon: "sparkles", tint: .pink, title: "LLM summary", subtitle: "qwen2.5:3b") {
                    Toggle("", isOn: $settings.config.llmSummary).labelsHidden()
                }
                .help("Summarize long \"on finish\" messages with the local Ollama model instead of a mid-sentence cutoff")
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
                    }
                }
            }

            VStack(spacing: 8) {
                Button {
                    runTest()
                } label: {
                    Label("Speak a Test Phrase", systemImage: "waveform")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.regular)

                Button("Quit") { NSApp.terminate(nil) }
                    .buttonStyle(.plain)
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
            }
        }
        .padding(14)
        .frame(width: 312)
    }

    private var header: some View {
        HStack(spacing: 8) {
            Image(systemName: "waveform")
                .font(.system(size: 13, weight: .bold))
                .foregroundStyle(.white)
                .frame(width: 26, height: 26)
                .background(Color.indigo.gradient, in: RoundedRectangle(cornerRadius: 7, style: .continuous))
            VStack(alignment: .leading, spacing: 0) {
                Text("Claude Voice").font(.system(size: 14, weight: .semibold))
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
                Slider(value: valueBinding, in: range).frame(width: 78).disabled(isUnlimited)
                Toggle("∞", isOn: unlimitedBinding)
                    .toggleStyle(.button)
                    .controlSize(.mini)
            }
        }
    }

    private func runTest() {
        settings.saveNow()
        runShell("\(VoicePaths.scriptsDir)/speak.sh", [
            "This is Claude Voice, speaking at the current speed, volume, and voice settings.",
        ])
    }

    private func previewVoice() {
        settings.saveNow()
        runShell("\(VoicePaths.scriptsDir)/speak.sh", [
            "This is the \(settings.config.voice) voice.",
            settings.config.voice,
        ])
    }

    private func dictate() {
        guard let bin = VoicePaths.dictateBinary else { return }
        let task = Process()
        task.executableURL = URL(fileURLWithPath: bin)
        task.arguments = ["-engine", "voxtral"]
        task.environment = VoicePaths.hardenedEnvironment
        try? task.run()
    }

    private func runShell(_ path: String, _ args: [String]) {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        task.arguments = [path] + args
        task.environment = VoicePaths.hardenedEnvironment
        try? task.run()
    }
}
