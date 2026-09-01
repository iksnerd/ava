import Foundation

/// Talks to the mlx-engine server (scripts/mlx-engine-server.sh) purely to show
/// status and offer start/stop from the menu — the hooks/speak.sh already
/// auto-start it on demand, this is just visibility + a manual override.
@MainActor
final class ServerController: ObservableObject {
    enum Status: Equatable {
        case unknown, running, stopped, starting, stopping
    }

    @Published var status: Status = .unknown

    private var pollTimer: Timer?

    init() {
        refresh()
        pollTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
    }

    // Periodic polling — deliberately backs off while a start/stop is in
    // flight so it doesn't fight with that operation's own status updates.
    func refresh() {
        Task {
            let up = await Self.health()
            if status != .starting && status != .stopping {
                status = up ? .running : .stopped
            }
        }
    }

    // Unconditionally resolves status from real health — used once a
    // start/stop's underlying process has actually finished, so it must
    // clear .starting/.stopping regardless of refresh()'s guard above
    // (which would otherwise block on the very state this is meant to end).
    private func forceRefresh() async {
        status = await Self.health() ? .running : .stopped
    }

    private static func health() async -> Bool {
        guard let url = URL(string: "http://127.0.0.1:8765/health") else { return false }
        var req = URLRequest(url: url)
        req.timeoutInterval = 2
        do {
            let (_, resp) = try await URLSession.shared.data(for: req)
            return (resp as? HTTPURLResponse)?.statusCode == 200
        } catch {
            return false
        }
    }

    func start() {
        status = .starting
        runScript("mlx-engine-server.sh", "start") { [weak self] _ in
            Task { @MainActor in await self?.forceRefresh() }
        }
    }

    func stop() {
        status = .stopping
        runScript("mlx-engine-server.sh", "stop") { [weak self] _ in
            Task { @MainActor in await self?.forceRefresh() }
        }
    }

    private func runScript(_ name: String, _ arg: String, completion: @escaping (Int32) -> Void) {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/bash")
        task.arguments = ["\(VoicePaths.scriptsDir)/\(name)", arg]
        task.environment = VoicePaths.hardenedEnvironment
        task.terminationHandler = { proc in
            DispatchQueue.main.async { completion(proc.terminationStatus) }
        }
        do {
            try task.run()
        } catch {
            DispatchQueue.main.async { completion(-1) }
        }
    }
}
