import Foundation
import Combine
import Darwin
import os

/// Manages the Go backend binary as a subprocess.
/// The binary serves the web UI on localhost and handles Google Messages protocol.
@MainActor
final class BackendManager: ObservableObject {
    enum State: Equatable {
        case stopped
        case starting
        case running
        case needsPairing
        case error(String)
    }

    @Published var state: State = .stopped
    @Published var port: Int = 7007

    private var process: Process?
    private let logger = Logger(subsystem: "com.openmessage.app", category: "Backend")
    private var healthCheckTask: Task<Void, Never>?
    private var connectionMonitorTask: Task<Void, Never>?

    /// Path to the embedded Go binary inside the app bundle.
    var binaryPath: String {
        if let resourcePath = Bundle.main.resourceURL?.appendingPathComponent("openmessage").path,
           FileManager.default.fileExists(atPath: resourcePath) {
            return resourcePath
        }
        if let executablePath = Bundle.main.executableURL?.deletingLastPathComponent().path {
            let embedded = (executablePath as NSString).appendingPathComponent("openmessage-helper")
            if FileManager.default.fileExists(atPath: embedded) {
                return embedded
            }
        }
        let systemPath = "/usr/local/bin/openmessage"
        if FileManager.default.fileExists(atPath: systemPath) {
            return systemPath
        }
        // Fallback: look next to the app or in a known dev location
        let devPath = FileManager.default.currentDirectoryPath + "/openmessage"
        if FileManager.default.fileExists(atPath: devPath) {
            return devPath
        }
        // Last resort: return the expected system install path
        return systemPath
    }

    /// Data directory for session, DB, etc.
    /// Uses Application Support inside the sandbox container, which is always writable.
    var dataDir: String {
        let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        let dir = appSupport.appendingPathComponent("OpenMessage").path
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true, attributes: nil)
        return dir
    }

    /// Migrate session and DB from old data dir (~/.local/share/openmessage) if present.
    private func migrateOldDataIfNeeded() {
        let oldDir = NSHomeDirectory() + "/.local/share/openmessage"
        let newDir = dataDir
        let fm = FileManager.default
        guard fm.fileExists(atPath: oldDir + "/session.json"),
              !fm.fileExists(atPath: newDir + "/session.json") else { return }
        for file in ["session.json", "messages.db", "messages.db-shm", "messages.db-wal"] {
            let src = oldDir + "/" + file
            let dst = newDir + "/" + file
            if fm.fileExists(atPath: src) {
                try? fm.copyItem(atPath: src, toPath: dst)
            }
        }
        logger.info("Migrated data from \(oldDir) to \(newDir)")
    }

    /// Whether a session file exists (i.e. phone is already paired).
    var hasSession: Bool {
        migrateOldDataIfNeeded()
        return FileManager.default.fileExists(atPath: dataDir + "/session.json")
    }

    var baseURL: URL {
        URL(string: "http://127.0.0.1:\(port)")!
    }

    func start() {
        guard state == .stopped || state == .needsPairing || state != .running else { return }

        if !hasSession {
            state = .needsPairing
            return
        }

        state = .starting
        cleanupConflictingBackendIfNeeded()
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: binaryPath)
        proc.arguments = ["serve"]
        proc.environment = [
            "OPENMESSAGES_PORT": String(port),
            "OPENMESSAGES_DATA_DIR": dataDir,
            "OPENMESSAGES_LOG_LEVEL": "info",
            "OPENMESSAGES_APP_SANDBOX": "1",
            "OPENMESSAGES_MACOS_NOTIFICATIONS": "1",
            "HOME": NSHomeDirectory(),
            "PATH": "/usr/local/bin:/usr/bin:/bin",
        ]

        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = pipe

        // Read output for logging
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            guard !data.isEmpty, let line = String(data: data, encoding: .utf8) else { return }
            self?.logger.info("\(line.trimmingCharacters(in: .whitespacesAndNewlines))")
        }

        proc.terminationHandler = { [weak self] proc in
            Task { @MainActor in
                guard let self else { return }
                self.logger.warning("Backend exited with code \(proc.terminationStatus)")
                if self.state == .running {
                    self.state = .error("Backend exited unexpectedly (code \(proc.terminationStatus))")
                }
            }
        }

        do {
            try proc.run()
            process = proc
            startHealthCheck()
        } catch {
            state = .error("Failed to launch backend: \(error.localizedDescription)")
            logger.error("Launch failed: \(error)")
        }
    }

    private func cleanupConflictingBackendIfNeeded() {
        let pids = listeningPIDs(on: port)
        guard !pids.isEmpty else { return }

        var terminatedAny = false
        for pid in pids {
            guard pid > 0 else { continue }
            guard let command = commandLine(for: pid) else { continue }
            guard command.contains("/usr/local/bin/openmessage serve")
                || command.contains("OpenMessage.app/Contents/Resources/openmessage serve")
            else { continue }

            logger.warning("Stopping conflicting backend pid \(pid): \(command, privacy: .public)")
            _ = Darwin.kill(pid_t(pid), SIGTERM)
            terminatedAny = true
        }

        if terminatedAny {
            Thread.sleep(forTimeInterval: 0.5)
        }
    }

    private func listeningPIDs(on port: Int) -> [Int32] {
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/sbin/lsof")
        proc.arguments = ["-ti", "tcp:\(port)", "-sTCP:LISTEN"]
        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = Pipe()

        do {
            try proc.run()
            proc.waitUntilExit()
            guard proc.terminationStatus == 0 else { return [] }
            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            let text = String(data: data, encoding: .utf8) ?? ""
            return text
                .split(whereSeparator: \.isNewline)
                .compactMap { Int32($0) }
        } catch {
            logger.error("Failed to inspect port \(port): \(error.localizedDescription, privacy: .public)")
            return []
        }
    }

    private func commandLine(for pid: Int32) -> String? {
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/bin/ps")
        proc.arguments = ["-o", "command=", "-p", String(pid)]
        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = Pipe()

        do {
            try proc.run()
            proc.waitUntilExit()
            guard proc.terminationStatus == 0 else { return nil }
            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            return String(data: data, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines)
        } catch {
            logger.error("Failed to inspect pid \(pid): \(error.localizedDescription, privacy: .public)")
            return nil
        }
    }

    func stop() {
        healthCheckTask?.cancel()
        healthCheckTask = nil
        connectionMonitorTask?.cancel()
        connectionMonitorTask = nil
        process?.terminate()
        process = nil
        state = .stopped
    }

    /// Poll /api/status until the backend is ready.
    private func startHealthCheck() {
        healthCheckTask = Task {
            for attempt in 1...30 {
                if Task.isCancelled { return }
                try? await Task.sleep(for: .milliseconds(500))
                do {
                    let url = baseURL.appendingPathComponent("api/status")
                    let (data, response) = try await URLSession.shared.data(from: url)
                    if let http = response as? HTTPURLResponse, http.statusCode == 200,
                       let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                        if json["connected"] as? Bool == true {
                            self.state = .running
                            self.logger.info("Backend ready after \(attempt) checks")
                            self.startConnectionMonitor()
                            return
                        }
                        if let google = json["google"] as? [String: Any],
                           google["needs_pairing"] as? Bool == true {
                            self.logger.warning("Backend reports Google Messages needs pairing")
                            self.stop()
                            self.state = .needsPairing
                            return
                        }
                    }
                } catch {
                    self.logger.debug("Health check \(attempt): \(error)")
                }
            }
            if !Task.isCancelled {
                self.state = .error("Backend failed to start within 15 seconds")
            }
        }
    }

    /// Periodically polls /api/status while running.
    /// If the backend reports disconnected, transitions back to needsPairing.
    private func startConnectionMonitor() {
        connectionMonitorTask = Task {
            var consecutiveDisconnects = 0
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(3))
                if Task.isCancelled { return }
                do {
                    let url = baseURL.appendingPathComponent("api/status")
                    let (data, response) = try await URLSession.shared.data(from: url)
                    if let http = response as? HTTPURLResponse, http.statusCode == 200,
                       let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                        let google = json["google"] as? [String: Any]
                        if google?["needs_pairing"] as? Bool == true {
                            self.logger.error("Google Messages session invalidated — returning to pairing")
                            self.handleDisconnect()
                            return
                        }
                        if json["connected"] as? Bool == false {
                            consecutiveDisconnects += 1
                            self.logger.warning("Disconnect detected (\(consecutiveDisconnects)/3)")
                            if consecutiveDisconnects >= 3 {
                                self.logger.error("Phone disconnected — returning to pairing")
                                self.handleDisconnect()
                                return
                            }
                        } else {
                            consecutiveDisconnects = 0
                        }
                    }
                } catch {
                    self.logger.debug("Connection monitor error: \(error)")
                }
            }
        }
    }

    /// Stop the backend, clean up session, and go back to pairing.
    private func handleDisconnect() {
        // Tell the backend to clean up (capture URL before stopping)
        let unpairURL = baseURL.appendingPathComponent("api/unpair")
        Task.detached {
            var request = URLRequest(url: unpairURL)
            request.httpMethod = "POST"
            _ = try? await URLSession.shared.data(for: request)
        }

        // Delete session file locally so pairing starts fresh
        let sessionPath = dataDir + "/session.json"
        try? FileManager.default.removeItem(atPath: sessionPath)

        // Stop the backend process and go to pairing
        stop()
        state = .needsPairing
    }

    /// Run the pairing flow. Returns the QR code URL for display.
    func startPairing() async -> AsyncStream<PairingEvent> {
        pairingEvents(arguments: ["pair"], stdinText: nil)
    }

    func startGooglePairing(cookieInput: String) async -> AsyncStream<PairingEvent> {
        pairingEvents(arguments: ["pair", "--google-stdin"], stdinText: cookieInput)
    }

    private func pairingEvents(arguments: [String], stdinText: String?) -> AsyncStream<PairingEvent> {
        let binPath = self.binaryPath
        let dataDirPath = self.dataDir
        return AsyncStream { continuation in
            Task.detached {
                let proc = Process()
                proc.executableURL = URL(fileURLWithPath: binPath)
                proc.arguments = arguments
                proc.environment = [
                    "OPENMESSAGES_DATA_DIR": dataDirPath,
                    "HOME": NSHomeDirectory(),
                    "PATH": "/usr/local/bin:/usr/bin:/bin",
                ]

                let pipe = Pipe()
                proc.standardOutput = pipe
                proc.standardError = pipe
                if let stdinText {
                    let stdinPipe = Pipe()
                    proc.standardInput = stdinPipe
                    stdinPipe.fileHandleForWriting.write(Data(stdinText.utf8))
                    stdinPipe.fileHandleForWriting.closeFile()
                }

                pipe.fileHandleForReading.readabilityHandler = { handle in
                    let data = handle.availableData
                    guard !data.isEmpty, let text = String(data: data, encoding: .utf8) else { return }

                    // Output may contain multiple lines (QR art + URL)
                    for line in text.components(separatedBy: .newlines) {
                        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
                        guard !trimmed.isEmpty else { continue }

                        // Extract URL from lines like "URL: https://..." or bare URLs
                        if let range = trimmed.range(of: "https://", options: .caseInsensitive) {
                            let url = String(trimmed[range.lowerBound...])
                            continuation.yield(.qrURL(url))
                        } else if trimmed.hasPrefix("EMOJI:") {
                            let emoji = trimmed.replacingOccurrences(of: "EMOJI:", with: "").trimmingCharacters(in: .whitespacesAndNewlines)
                            continuation.yield(.emoji(emoji))
                        } else if trimmed.hasPrefix("http://") {
                            continuation.yield(.qrURL(trimmed))
                        } else if trimmed.lowercased().contains("success") || trimmed.lowercased().contains("paired") {
                            continuation.yield(.success)
                        } else if !trimmed.contains("█") && !trimmed.contains("▀") && !trimmed.contains("▄") {
                            continuation.yield(.log(trimmed))
                        }
                        // Skip QR art and other log lines to avoid noisy status updates
                    }
                }

                proc.terminationHandler = { proc in
                    if proc.terminationStatus == 0 {
                        continuation.yield(.success)
                    } else {
                        continuation.yield(.failed("Pairing exited with code \(proc.terminationStatus)"))
                    }
                    continuation.finish()
                }

                do {
                    try proc.run()
                } catch {
                    continuation.yield(.failed("Could not start pairing: \(error.localizedDescription)"))
                    continuation.finish()
                }
            }
        }
    }

    deinit {
        process?.terminate()
    }
}

enum PairingEvent {
    case qrURL(String)
    case emoji(String)
    case log(String)
    case success
    case failed(String)
}
