import Foundation

/// Offers `ava setup` from the menu bar, so someone who installed only the app
/// never needs Terminal for it, Homebrew aside. Whether to offer it comes from
/// `ava setup --check`, which uses setup's own rules for skipping a step, so
/// the panel and the command cannot disagree about what is missing.
@MainActor
final class SetupController: ObservableObject {
    enum State: Equatable {
        case checking, complete, incomplete, running, failed
    }

    @Published private(set) var state: State = .checking
    /// What `--check` reported missing, e.g. "the Kokoro model".
    @Published private(set) var missing: [String] = []
    /// The latest line `ava setup` printed, so a long download visibly moves.
    @Published private(set) var progress = ""
    @Published private(set) var failure: String?

    /// setupTools' own wording when it cannot install the tools.
    var needsHomebrew: Bool { failure?.contains("Homebrew is not installed") ?? false }

    init() {
        check()
    }

    func check() {
        guard let ava = VoicePaths.dictateBinary else {
            state = .failed
            failure = "The ava binary is missing from this app. Reinstall Ava."
            return
        }
        state = .checking
        Self.run(ava, ["setup", "--check"], onLine: { _ in }) { [weak self] code, lines in
            guard let self else { return }
            self.missing = lines.compactMap { line in
                line.hasPrefix("❌ Missing: ") ? String(line.dropFirst("❌ Missing: ".count)) : nil
            }
            self.state = code == 0 ? .complete : .incomplete
        }
    }

    func setUp() {
        guard let ava = VoicePaths.dictateBinary else { return }
        state = .running
        failure = nil
        progress = "Starting…"
        Self.run(ava, ["setup"], onLine: { [weak self] line in
            self?.progress = line
        }) { [weak self] code, lines in
            guard let self else { return }
            if code == 0 {
                self.check()
                return
            }
            // ava prints its error last, prefixed ❌; the lines before it are
            // brew's and uv's progress.
            let reason = lines.last { $0.hasPrefix("❌") } ?? lines.last ?? "exit status \(code)"
            self.failure = reason.hasPrefix("❌ ") ? String(reason.dropFirst(2)) : reason
            self.state = .failed
        }
    }

    /// Runs ava with stdout and stderr merged, handing each non-empty line to
    /// onLine as it arrives and every line to done at the end, on the main actor.
    private static func run(
        _ ava: String, _ args: [String],
        onLine: @escaping @MainActor (String) -> Void,
        done: @escaping @MainActor (Int32, [String]) -> Void
    ) {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: ava)
        task.arguments = args
        task.environment = VoicePaths.hardenedEnvironment
        let pipe = Pipe()
        task.standardOutput = pipe
        task.standardError = pipe

        let collector = LineCollector()
        pipe.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            guard !data.isEmpty else { return }
            for line in collector.append(data) {
                Task { @MainActor in onLine(line) }
            }
        }
        task.terminationHandler = { proc in
            pipe.fileHandleForReading.readabilityHandler = nil
            let rest = pipe.fileHandleForReading.readDataToEndOfFile()
            _ = collector.append(rest)
            let lines = collector.finish()
            Task { @MainActor in done(proc.terminationStatus, lines) }
        }
        do {
            try task.run()
        } catch {
            Task { @MainActor in done(-1, ["Could not run ava: \(error.localizedDescription)"]) }
        }
    }
}

/// Splits a byte stream into trimmed, non-empty lines. brew and uv redraw
/// progress with carriage returns, so those end a line too.
private final class LineCollector: @unchecked Sendable {
    private let lock = NSLock()
    private var partial = ""
    private var all: [String] = []

    func append(_ data: Data) -> [String] {
        lock.lock()
        defer { lock.unlock() }
        partial += String(decoding: data, as: UTF8.self)
        var complete = partial.components(separatedBy: CharacterSet(charactersIn: "\r\n"))
        partial = complete.removeLast()
        let lines = complete.map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        all += lines
        return lines
    }

    func finish() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        let last = partial.trimmingCharacters(in: .whitespaces)
        if !last.isEmpty { all.append(last) }
        partial = ""
        return all
    }
}
