import AppKit
import SwiftUI

@main
struct ClaudeVoiceMenuBarApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var settings = VoiceSettings()

    var body: some Scene {
        MenuBarExtra("Claude Voice", systemImage: "waveform") {
            SettingsView()
                .environmentObject(settings)
        }
        .menuBarExtraStyle(.window)
    }
}

/// Running as a bare SPM executable (no Info.plist/LSUIElement), so hide the
/// Dock icon in code instead — this is what actually makes it a menu bar app.
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
    }
}
