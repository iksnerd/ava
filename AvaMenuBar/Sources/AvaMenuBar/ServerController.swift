import Foundation

/// Talks to the mlx-engine server (scripts/mlx-engine-server.sh) purely to show
/// status and offer start/stop from the menu — the hooks/speak.sh already
/// auto-start it on demand, this is just visibility + a manual override.
/// stop() is sticky: mlx-engine-server.sh itself disarms that on-demand
/// auto-start (engineAutoStart in VoiceConfig) so a hook firing right after
/// doesn't silently bring the server back up; start() re-arms it.
@MainActor
final class ServerController: ObservableObject {
    enum Status: Equatable {
        case unknown, running, stopped, starting, stopping
    }

    @Published var status: Status = .unknown

    /// The last thing that went wrong, for the panel to show. Start failing
    /// used to be completely silent: runScript handed the exit code to a
    /// completion that discarded it with `_`, so on a machine without
    /// mlx-engine/ installed the badge simply returned to "Stopped" and the
    /// user pressed the button again.
    @Published var lastError: String?

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
        guard let url = URL(string: "\(AvaProtocol.engineURL)/health") else { return false }
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
        lastError = nil
        runScript("mlx-engine-server.sh", "start") { [weak self] code in
            Task { @MainActor in
                self?.report(code, verb: "start")
                await self?.forceRefresh()
            }
        }
    }

    func stop() {
        status = .stopping
        lastError = nil
        runScript("mlx-engine-server.sh", "stop") { [weak self] code in
            Task { @MainActor in
                self?.report(code, verb: "stop")
                await self?.forceRefresh()
            }
        }
    }

    /// Turns a non-zero exit into something the panel can show. -1 is
    /// runScript's own marker for "the process never launched", which is what
    /// happens when the bundled scripts are missing entirely.
    private func report(_ code: Int32, verb: String) {
        guard code != 0 else { return }
        lastError = code == -1
            ? "Could not run the \(verb) script. Reinstall the app, or check \(VoicePaths.scriptsDir)."
            : "Server \(verb) failed (exit \(code)). Is mlx-engine set up? Run `make setup-deps` in the repo, or see /tmp/mlx-engine-server.log."
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
