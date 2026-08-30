import Foundation

/// Polls scripts/speak.sh's per-invocation marker directory (one file per
/// in-flight speak, present for the full synth+playback duration) so the
/// menu bar icon can show when Claude Voice is actively speaking — this
/// covers speech triggered from anywhere (Read Aloud, Claude Code hooks,
/// the Test/Preview buttons), not just actions taken in this app.
@MainActor
final class SpeechActivityMonitor: ObservableObject {
    @Published private(set) var isSpeaking = false

    private static let activityDir = "/tmp/claude-tts-active"

    private var timer: Timer?

    init() {
        poll()
        timer = Timer.scheduledTimer(withTimeInterval: 0.3, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.poll() }
        }
    }

    private func poll() {
        let contents = try? FileManager.default.contentsOfDirectory(atPath: Self.activityDir)
        isSpeaking = !(contents?.isEmpty ?? true)
    }
}
