import Foundation

/// Single source of truth for where the shell side of this stack lives, so a
/// repo move only needs updating here instead of in every file that shells out.
enum VoicePaths {
    static let scriptsDir = "/Users/user/GolandProjects/local-whisper/scripts"
}
