import Foundation

/// Mirrors the JSON shape read by scripts/lib.sh's config_get — property
/// names double as the JSON keys, so keep them in sync with the shell side.
struct VoiceConfig: Codable, Equatable {
    var muted: Bool = false
    var speed: Double = 1.3
    var volume: Double = 1.0
    var voice: String = "af_heart"
    var sayRate: Int = 220
    var stopMaxChars: Int = 600
    var notifyMaxChars: Int = 500
    var llmSummary: Bool = false
    // False once the user has explicitly stopped the mlx-engine server
    // (Stop below, or `ava engine stop`) — until they explicitly
    // start it again, this tells speak.sh's on-demand auto-start (hooks)
    // not to silently bring the server back up. Flipped by
    // scripts/mlx-engine-server.sh itself (see config_set_bool in lib.sh),
    // not by this app directly — ServerController.stop()/start() just run
    // that script, and VoiceSettings picks the resulting change up like any
    // other external edit (reloadIfChangedOnDisk).
    var engineAutoStart: Bool = true
    // Read so the UI can name the model it will actually use and check that
    // Ollama has it (OllamaStatus). Only scripts/hook-stop.sh consumes the
    // value itself, via lib.sh's generic config_get.
    var summaryModel: String = "qwen2.5:3b"
}

// Decoded key by key, each bad value costing only its own key.
//
// The synthesized Codable init is all-or-nothing: one wrong-typed value
// anywhere in the file — a hand-edited "speed": "fast" — fails the whole
// decode, and load() below then falls back to *every* property default,
// including muted = false. The two other readers of this same file,
// scripts/lib.sh's config_get and internal/voiceconfig, both degrade per
// key, so before this the menu bar app could show unmuted and speak while
// the hooks and the CLI stayed correctly silent. Mute is the one setting
// whose entire job is to be absolute.
//
// Declared in an extension on purpose: an init(from:) inside the struct
// body would suppress the synthesized init(), which load() returns when
// both files are unreadable.
extension VoiceConfig {
    init(from decoder: Decoder) throws {
        self.init()
        guard let c = try? decoder.container(keyedBy: CodingKeys.self) else { return }
        muted = Self.lenient(c, .muted) ?? muted
        speed = Self.lenient(c, .speed) ?? speed
        volume = Self.lenient(c, .volume) ?? volume
        voice = Self.lenient(c, .voice) ?? voice
        sayRate = Self.lenient(c, .sayRate) ?? sayRate
        stopMaxChars = Self.lenient(c, .stopMaxChars) ?? stopMaxChars
        notifyMaxChars = Self.lenient(c, .notifyMaxChars) ?? notifyMaxChars
        llmSummary = Self.lenient(c, .llmSummary) ?? llmSummary
        engineAutoStart = Self.lenient(c, .engineAutoStart) ?? engineAutoStart
        summaryModel = Self.lenient(c, .summaryModel) ?? summaryModel
    }

    /// nil for a key that is absent, of the wrong type, or otherwise
    /// undecodable — the caller keeps its existing value in all three cases.
    private static func lenient<T: Decodable>(_ c: KeyedDecodingContainer<CodingKeys>, _ key: CodingKeys) -> T? {
        guard let value = try? c.decodeIfPresent(T.self, forKey: key) else { return nil }
        return value
    }
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

    // nonisolated: read/written from toggleMutedOnDisk(), which runs on the
    // Services callback thread (not necessarily MainActor) via SpeechService.
    nonisolated static let fileURL: URL = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let dir = base.appendingPathComponent("ava", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("config.json")
    }()

    // Same file scripts/lib.sh's config_get falls back to — the single
    // source of truth for defaults, so they only need to change in one place.
    nonisolated static let defaultsFileURL = URL(fileURLWithPath: VoicePaths.scriptsDir)
        .appendingPathComponent("voice-defaults.json")

    private var saveWorkItem: DispatchWorkItem?
    private var externalChangeTimer: Timer?
    private var lastKnownModDate: Date?

    init() {
        config = Self.load()
        lastKnownModDate = Self.modDate()
        // Mute (and anything else) can also be flipped from outside this
        // process — e.g. the "Toggle Ava Mute" Service, which has
        // no reference to this live instance and writes straight to disk —
        // so poll for that the same way SpeechActivityMonitor polls its
        // activity directory, rather than only ever trusting our own writes.
        externalChangeTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.reloadIfChangedOnDisk() }
        }
    }

    private static func modDate() -> Date? {
        (try? FileManager.default.attributesOfItem(atPath: fileURL.path))?[.modificationDate] as? Date
    }

    private func reloadIfChangedOnDisk() {
        let modDate = Self.modDate()
        guard modDate != lastKnownModDate else { return }
        lastKnownModDate = modDate
        let onDisk = Self.load()
        // Our own scheduleSave() writes exactly what's already in `config`,
        // so this only actually reassigns (and re-triggers didSet/save) for
        // a genuinely external change — no feedback loop with our own saves.
        guard onDisk != config else { return }
        config = onDisk
    }

    // Merges voice-defaults.json (base) with the live config (overlay) key
    // by key, so a live config missing a newer field picks it up from the
    // shared defaults. That covers a *missing* key; a present-but-wrong-typed
    // one is handled by the tolerant init(from:) above, which is what
    // actually mirrors config_get's per-key fallback. The struct's own
    // property defaults are a last resort if *both* files are unreadable.
    nonisolated static func load() -> VoiceConfig {
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

    private nonisolated static func readJSONObject(_ url: URL) -> [String: Any]? {
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

    /// Flips `muted` directly on disk and returns the new value — used by
    /// the "Toggle Ava Mute" Service (SpeechService.swift), which
    /// has no reference to the live instance the SwiftUI app owns. That
    /// instance picks the change up via its own poll above.
    @discardableResult
    nonisolated static func toggleMutedOnDisk() -> Bool {
        var cfg = load()
        cfg.muted.toggle()
        if let data = try? JSONEncoder().encode(cfg) {
            try? data.write(to: fileURL, options: .atomic)
        }
        return cfg.muted
    }
}
