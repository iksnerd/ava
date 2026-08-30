import AppKit

/// Backs "Read Aloud with Claude Voice" — the item macOS adds to the Services
/// submenu of the right-click menu for selected text in any app. Declared in
/// Info.plist's NSServices (scripts/build-app.sh) and wired up as
/// NSApp.servicesProvider in ClaudeVoiceMenuBarApp's AppDelegate; the
/// `readAloud` selector name here must match NSMessage in that plist entry.
final class SpeechService: NSObject {
    @objc func readAloud(
        _ pasteboard: NSPasteboard, userData: String, error: AutoreleasingUnsafeMutablePointer<NSString>
    ) {
        guard let text = pasteboard.string(forType: .string), !text.isEmpty else {
            error.pointee = "No text was selected." as NSString
            return
        }
        Speech.speak(text)
    }
}
