// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ClaudeVoiceMenuBar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "ClaudeVoiceMenuBar",
            path: "Sources/ClaudeVoiceMenuBar"
        )
    ]
)
