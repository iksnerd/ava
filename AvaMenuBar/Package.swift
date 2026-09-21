// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "AvaMenuBar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "AvaMenuBar",
            path: "Sources/AvaMenuBar"
        )
    ]
)
