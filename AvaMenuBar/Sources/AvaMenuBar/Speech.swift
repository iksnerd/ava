import Foundation

/// Shells out to scripts/speak.sh — shared by the Settings UI (Test/Preview
/// buttons) and SpeechService (the right-click "Read Aloud" Service).
enum Speech {
    /// Set when a launch fails, for the panel to show. These used to be `try?`:
    /// a missing or non-executable script meant Test and Read Aloud did
    /// precisely nothing, with no error anywhere. The README already records
    /// this failure class biting once via PATH; the PATH cause was fixed, the
    /// silence was not.
    @MainActor static var lastError: String?

    static func speak(_ text: String, voice: String? = nil) {
        guard !text.isEmpty else { return }
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        var args = ["\(VoicePaths.scriptsDir)/speak.sh", text]
        if let voice { args.append(voice) }
        task.arguments = args
        task.environment = VoicePaths.hardenedEnvironment
        run(task, what: "speak.sh")
    }

    /// Cancels whatever speak.sh is currently doing — synthesis in flight or
    /// audio already playing — rather than letting it run to completion.
    static func stop() {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        task.arguments = ["\(VoicePaths.scriptsDir)/stop-speaking.sh"]
        task.environment = VoicePaths.hardenedEnvironment
        run(task, what: "stop-speaking.sh")
    }

    private static func run(_ task: Process, what: String) {
        do {
            try task.run()
            Task { @MainActor in lastError = nil }
        } catch {
            let message = "Could not run \(what): \(error.localizedDescription)"
            Task { @MainActor in lastError = message }
        }
    }
}
