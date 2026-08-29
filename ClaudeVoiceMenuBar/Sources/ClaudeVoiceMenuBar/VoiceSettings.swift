import Foundation

/// Mirrors the JSON shape read by scripts/lib.sh's config_get — property
/// names double as the JSON keys, so keep them in sync with the shell side.
struct VoiceConfig: Codable, Equatable {
    var speed: Double = 1.3
    var volume: Double = 1.0
    var voice: String = "af_heart"
    var sayRate: Int = 220
    var stopMaxChars: Int = 600
    var notifyMaxChars: Int = 500
    var llmSummary: Bool = false
}

/// English Kokoro voices bundled in the already-downloaded
/// mlx-community/Kokoro-82M-bf16 snapshot (~/.cache/huggingface/.../voices).
enum KokoroVoice {
    static let english: [String] = [
        "af_alloy", "af_aoede", "af_bella", "af_heart", "af_jessica", "af_kore",
        "af_nicole", "af_nova", "af_river", "af_sarah", "af_sky",
        "am_adam", "am_echo", "am_eric", "am_fenrir", "am_liam", "am_michael",
        "am_onyx", "am_puck", "am_santa",
        "bf_alice", "bf_emma", "bf_isabella", "bf_lily",
        "bm_daniel", "bm_fable", "bm_george", "bm_lewis",
    ]
}

@MainActor
final class VoiceSettings: ObservableObject {
    @Published var config: VoiceConfig {
        didSet {
            guard config != oldValue else { return }
            scheduleSave()
        }
    }

    static let fileURL: URL = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let dir = base.appendingPathComponent("ClaudeVoice", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("config.json")
    }()

    // Same file scripts/lib.sh's config_get falls back to — the single
    // source of truth for defaults, so they only need to change in one place.
    static let defaultsFileURL = URL(fileURLWithPath: VoicePaths.scriptsDir)
        .appendingPathComponent("voice-defaults.json")

    private var saveWorkItem: DispatchWorkItem?

    init() {
        config = Self.load()
    }

    // Merges voice-defaults.json (base) with the live config (overlay) key
    // by key, mirroring config_get's per-key fallback — so a live config
    // missing a newer field still picks it up from the shared defaults
    // instead of the whole decode failing. The struct's own property
    // defaults are a last resort if *both* files are unreadable.
    static func load() -> VoiceConfig {
        var merged = readJSONObject(defaultsFileURL) ?? [:]
        if let live = readJSONObject(fileURL) {
            for (key, value) in live { merged[key] = value }
        }
        guard !merged.isEmpty,
              let data = try? JSONSerialization.data(withJSONObject: merged),
              let cfg = try? JSONDecoder().decode(VoiceConfig.self, from: data)
        else {
            return VoiceConfig()
        }
        return cfg
    }

    private static func readJSONObject(_ url: URL) -> [String: Any]? {
        guard let data = try? Data(contentsOf: url) else { return nil }
        return try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    }

    // Debounced so dragging a slider doesn't hammer disk with every tick.
    private func scheduleSave() {
        saveWorkItem?.cancel()
        let item = DispatchWorkItem { [weak self] in self?.save() }
        saveWorkItem = item
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.25, execute: item)
    }

    func save() {
        guard let data = try? JSONEncoder().encode(config) else { return }
        try? data.write(to: Self.fileURL, options: .atomic)
    }

    /// Flushes any pending debounced save immediately — use before shelling
    /// out to speak.sh so a Test press reflects the value just dragged.
    func saveNow() {
        saveWorkItem?.cancel()
        save()
    }
}
