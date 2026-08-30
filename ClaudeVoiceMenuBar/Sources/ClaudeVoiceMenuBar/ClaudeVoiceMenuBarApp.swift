import AppKit
import SwiftUI

@main
struct ClaudeVoiceMenuBarApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var settings = VoiceSettings()
    @StateObject private var activity = SpeechActivityMonitor()

    var body: some Scene {
        MenuBarExtra {
            SettingsView()
                .environmentObject(settings)
                .environmentObject(activity)
        } label: {
            Label("Claude Voice", systemImage: activity.isSpeaking ? "waveform.circle.fill" : "waveform")
        }
        .menuBarExtraStyle(.window)
    }
}

/// Running as a bare SPM executable (no Info.plist/LSUIElement), so hide the
/// Dock icon in code instead — this is what actually makes it a menu bar app.
final class AppDelegate: NSObject, NSApplicationDelegate {
    private let speechService = SpeechService()

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        NSApp.servicesProvider = speechService
        // Forces macOS to re-scan Info.plist's NSServices now rather than
        // waiting for its own cache to notice — otherwise a freshly
        // (re)installed .app's Services-menu entry can take a while to
        // show up in other apps' right-click menus.
        NSUpdateDynamicServices()
    }
}
