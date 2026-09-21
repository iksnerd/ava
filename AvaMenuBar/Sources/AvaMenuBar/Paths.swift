import Foundation

/// Single source of truth for where the shell side of this stack lives, so a
/// repo move only needs updating here instead of in every file that shells out.
enum VoicePaths {
    // Only used as a dev-checkout fallback (below) when this isn't running
    // from a properly packaged .app — e.g. `swift run` during development.
    // A packaged build resolves scriptsDir/dictateBinary from its own
    // bundled Resources/ instead (see bundleResourcesDir).
    //
    // Derived from this file's own compile-time location rather than written
    // out, so it follows the checkout it was built from instead of only ever
    // being right on one machine. #filePath is
    // <repo>/AvaMenuBar/Sources/AvaMenuBar/Paths.swift, hence
    // four levels up.
    private static let devRepoRoot: String = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()  // AvaMenuBar/
        .deletingLastPathComponent()  // Sources/
        .deletingLastPathComponent()  // AvaMenuBar/
        .deletingLastPathComponent()  // repo root
        .path

    // Bundle.main.resourceURL when running from a real .app with a bundled
    // Resources/scripts (build-app.sh copies scripts/ + local-whisper there)
    // — nil for `swift run`, whose Bundle.main has no such Resources/, so
    // callers below fall through to the dev-checkout paths instead.
    private static let bundleResourcesDir: URL? = {
        guard let url = Bundle.main.resourceURL,
            FileManager.default.fileExists(atPath: url.appendingPathComponent("scripts").path)
        else { return nil }
        return url
    }()

    static var scriptsDir: String {
        if let dir = bundleResourcesDir {
            return dir.appendingPathComponent("scripts").path
        }
        return "\(devRepoRoot)/scripts"
    }

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

    /// Prefers a packaged build's own bundled binary (build-app.sh copies it
    /// into Resources/), then the repo-local `bin/` build output (`make
    /// build`, for `swift run` dev usage), then the `make install-bin`
    /// location (~/.local/bin) — an `~/.local/bin` install can silently go
    /// stale (predating a flag this app passes, for instance) since nothing
    /// prompts a rebuild of it, so it's only ever the last resort.
    static var dictateBinary: String? {
        if let dir = bundleResourcesDir {
            let bundled = dir.appendingPathComponent("local-whisper").path
            if FileManager.default.isExecutableFile(atPath: bundled) { return bundled }
        }
        let local = "\(devRepoRoot)/bin/local-whisper"
        if FileManager.default.isExecutableFile(atPath: local) { return local }
        let installed = NSString(string: "~/.local/bin/local-whisper").expandingTildeInPath
        if FileManager.default.isExecutableFile(atPath: installed) { return installed }
        return nil
    }
}
