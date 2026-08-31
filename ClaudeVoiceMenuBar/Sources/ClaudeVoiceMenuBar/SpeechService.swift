import AppKit

/// Backs the two system Services this app registers via Info.plist's
/// NSServices (scripts/build-app.sh) and NSApp.servicesProvider in
/// ClaudeVoiceMenuBarApp's AppDelegate — each `@objc` method name here must
/// match an NSMessage entry in that plist.
final class SpeechService: NSObject {
    /// "Read Aloud with Claude Voice" — appears in the Services submenu of
    /// the right-click menu for selected text in any app that supports it.
    @objc func readAloud(
        _ pasteboard: NSPasteboard, userData: String, error: AutoreleasingUnsafeMutablePointer<NSString>
    ) {
        guard let text = pasteboard.string(forType: .string), !text.isEmpty else {
            error.pointee = "No text was selected." as NSString
            return
        }
        Speech.speak(text)
    }

    /// "Toggle Claude Voice Mute" — an action Service with no send/return
    /// pasteboard types, so it needs no selected text and shows up under
    /// System Settings → Keyboard → Keyboard Shortcuts → Services → General,
    /// where it can be bound to a global keyboard shortcut to mute/unmute
    /// from anywhere without opening the menu bar panel. Writes straight to
    /// config.json (VoiceSettings' own live instance picks the change up via
    /// its poll) and plays a system sound rather than speaking, since a
    /// spoken "muted" confirmation would defeat the point.
    @objc func toggleMute(
        _ pasteboard: NSPasteboard, userData: String, error: AutoreleasingUnsafeMutablePointer<NSString>
    ) {
        let muted = VoiceSettings.toggleMutedOnDisk()
        if muted { Speech.stop() }
        NSSound(named: muted ? "Tink" : "Pop")?.play()
    }
}
