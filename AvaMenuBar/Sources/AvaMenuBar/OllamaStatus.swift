import Foundation

/// Checks that the LLM-summary setting can actually do anything.
///
/// Turning `llmSummary` on used to be unverifiable from the UI: the hook asks
/// Ollama for a summary, gets nothing back if the daemon is down or the model was
/// never pulled, and falls back to truncating — correct behaviour for a hook,
/// which must never fail a session, but the user is told nothing and simply
/// never hears a summary. The subtitle also hardcoded a model name rather than
/// reading the one in the config.
@MainActor
final class OllamaStatus: ObservableObject {
    enum State: Equatable {
        case idle           // llmSummary is off; nothing to check
        case checking
        case ready(model: String)
        case daemonDown
        case modelMissing(model: String)

        /// nil when there is nothing wrong worth saying.
        var problem: String? {
            switch self {
            case .idle, .checking, .ready:
                return nil
            case .daemonDown:
                return "Ollama isn't reachable on 127.0.0.1:11434 — long messages will be cut off mid-sentence instead of summarized. Start Ollama, or turn this off."
            case .modelMissing(let model):
                return "Ollama is running but doesn't have \(model) — long messages will be cut off instead of summarized. Run `ollama pull \(model)`."
            }
        }
    }

    @Published private(set) var state: State = .idle

    private var pollTimer: Timer?

    /// Re-check whenever the setting or the model changes, and periodically
    /// while it is on — Ollama is a separate daemon that can stop at any time.
    func track(enabled: Bool, model: String) {
        pollTimer?.invalidate()
        pollTimer = nil

        guard enabled else {
            state = .idle
            return
        }
        Task { await check(model: model) }
        pollTimer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.check(model: model) }
        }
    }

    private func check(model: String) async {
        if case .idle = state { state = .checking }

        guard let url = URL(string: "http://127.0.0.1:11434/api/tags") else {
            state = .daemonDown
            return
        }
        var req = URLRequest(url: url)
        req.timeoutInterval = 3

        do {
            let (data, resp) = try await URLSession.shared.data(for: req)
            guard (resp as? HTTPURLResponse)?.statusCode == 200 else {
                state = .daemonDown
                return
            }
            state = Self.hasModel(model, in: data) ? .ready(model: model) : .modelMissing(model: model)
        } catch {
            state = .daemonDown
        }
    }

    /// Ollama reports `{"models":[{"name":"qwen2.5:3b", ...}]}`. A config value
    /// with no tag (`qwen2.5`) should match the `:latest` Ollama reports, so
    /// compare on the base name when the config gives no tag.
    static func hasModel(_ wanted: String, in tagsJSON: Data) -> Bool {
        guard
            let root = try? JSONSerialization.jsonObject(with: tagsJSON) as? [String: Any],
            let models = root["models"] as? [[String: Any]]
        else { return false }

        let names = models.compactMap { $0["name"] as? String }
        if names.contains(wanted) { return true }
        if !wanted.contains(":") {
            return names.contains { $0.split(separator: ":").first.map(String.init) == wanted }
        }
        return false
    }
}
