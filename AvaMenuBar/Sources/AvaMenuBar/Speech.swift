import Foundation

/// Shells out to scripts/speak.sh — shared by the Settings UI (Test/Preview
/// buttons) and SpeechService (the right-click "Read Aloud" Service).
enum Speech {
    static func speak(_ text: String, voice: String? = nil) {
        guard !text.isEmpty else { return }
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        var args = ["\(VoicePaths.scriptsDir)/speak.sh", text]
        if let voice { args.append(voice) }
        task.arguments = args
        task.environment = VoicePaths.hardenedEnvironment
        try? task.run()
    }

    /// Cancels whatever speak.sh is currently doing — synthesis in flight or
    /// audio already playing — rather than letting it run to completion.
    static func stop() {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        task.arguments = ["\(VoicePaths.scriptsDir)/stop-speaking.sh"]
        task.environment = VoicePaths.hardenedEnvironment
        try? task.run()
    }
}
