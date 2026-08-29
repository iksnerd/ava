import Foundation

/// Single source of truth for where the shell side of this stack lives, so a
/// repo move only needs updating here instead of in every file that shells out.
enum VoicePaths {
    static let scriptsDir = "/Users/user/GolandProjects/local-whisper/scripts"
    static let repoRoot = "/Users/user/GolandProjects/local-whisper"

    /// A GUI app launched via LaunchServices/launchd inherits a minimal PATH
    /// (just /usr/bin:/bin:/usr/sbin:/sbin — confirmed via `ps eww`/`launchctl
    /// print` on this app's own running process), not the interactive shell's
    /// PATH. Any subprocess this app spawns directly (not going through a
    /// script that sources lib.sh, which hardens its own PATH already) needs
    /// this set explicitly, or dependency lookups like `exec.LookPath("sox")`
    /// silently fail and the process exits before doing anything.
    static var hardenedEnvironment: [String: String] {
        var env = ProcessInfo.processInfo.environment
        let extra = "/opt/homebrew/bin:/usr/local/bin:\(NSString(string: "~/.local/bin").expandingTildeInPath)"
        env["PATH"] = "\(extra):\(env["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin")"
        return env
    }

    /// Prefers the repo-local `bin/` build output (`make build`) over the
    /// `make install-bin` location (~/.local/bin) — this app lives in the
    /// same repo, so its own sibling build is more likely to be current;
    /// an `~/.local/bin` install can silently go stale (predating a flag
    /// this app passes, for instance) since nothing prompts a rebuild of it.
    static var dictateBinary: String? {
        let local = "\(repoRoot)/bin/local-whisper"
        if FileManager.default.isExecutableFile(atPath: local) { return local }
        let installed = NSString(string: "~/.local/bin/local-whisper").expandingTildeInPath
        if FileManager.default.isExecutableFile(atPath: installed) { return installed }
        return nil
    }
}
