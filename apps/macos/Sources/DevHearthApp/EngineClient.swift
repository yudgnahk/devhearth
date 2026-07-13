import Foundation
import Observation

enum EngineClientError: LocalizedError {
    case engineNotFound(searched: [String])
    case invalidMessage(detail: String)
    case incompatibleEngine
    case incompleteStream
    case engineNotRunning
    case engineExited(code: Int32)
    case engineRPC(RPCError)

    var errorDescription: String? {
        switch self {
        case .engineNotFound(let searched):
            let paths = searched.joined(separator: "\n  ")
            return "Could not find the DevHearth engine. Set DEVHEARTH_ENGINE_PATH or run `make build`.\nSearched:\n  \(paths)"
        case .invalidMessage(let detail):
            return "Invalid engine message: \(detail)"
        case .incompatibleEngine:
            return "The engine protocol or schema is incompatible."
        case .incompleteStream:
            return "The engine closed before the scan completed."
        case .engineNotRunning:
            return "The engine is not running."
        case .engineExited(let code):
            return "The engine exited unexpectedly (code \(code))."
        case .engineRPC(let error):
            return error.localizedDescription
        }
    }
}

@MainActor
@Observable
final class EngineClient {
    private(set) var status = "Starting…"
    private(set) var progress: [ScanProgress] = []
    private(set) var assets: [AssetSummary] = []
    private(set) var relationships: [RelationshipSummary] = []
    private(set) var portfolio: [PortfolioSummary] = []
    private(set) var errorMessage: String?
    private(set) var isScanning = false
    private(set) var enginePath: String?

    private var process: Process?
    private var stdinHandle: FileHandle?
    private var nextID = 0
    private var helloRequestID: String?
    private var scanRequestID: String?
    private var assetsRequestID: String?
    private var portfolioRequestID: String?
    private var activeScanID: String?
    private var scanRoots: [String] = []
    private var assetsLoaded = false
    private var portfolioLoaded = false
    private var mode: SessionMode = .idle
    private var connectTask: Task<Void, Never>?

    private enum SessionMode {
        case idle
        case probe
        case scan
    }

    /// Locate and hello-negotiate the engine without starting a scan.
    func connect() async {
        if let connectTask {
            await connectTask.value
            return
        }
        let task = Task { @MainActor in
            await self.performConnect()
        }
        connectTask = task
        await task.value
        connectTask = nil
    }

    private func performConnect() async {
        guard !isScanning else { return }
        do {
            errorMessage = nil
            status = "Locating engine…"
            let executable = try resolveEngineURL()
            enginePath = executable.path
            try await runSession(mode: .probe, roots: [])
            status = "Engine ready · choose a folder to scan"
        } catch {
            errorMessage = error.localizedDescription
            status = statusForFailure(error, duringScan: false)
        }
    }

    func start(roots: [String]) async {
        // Avoid overlapping a probe connect with a scan.
        if let connectTask {
            await connectTask.value
        }
        do {
            progress = []
            assets = []
            relationships = []
            portfolio = []
            errorMessage = nil
            isScanning = true
            assetsLoaded = false
            portfolioLoaded = false
            scanRoots = roots
            status = "Starting scan…"
            _ = try resolveEngineURL()
            try await runSession(mode: .scan, roots: roots)
            if progress.last?.complete != true {
                throw EngineClientError.incompleteStream
            }
        } catch {
            errorMessage = error.localizedDescription
            status = statusForFailure(error, duringScan: true)
        }
        isScanning = false
    }

    func stop() {
        if let handle = stdinHandle {
            try? handle.close()
        }
        stdinHandle = nil
        if let process, process.isRunning {
            process.terminate()
            process.waitUntilExit()
        }
        process = nil
        mode = .idle
    }

    func cancelScan() {
        guard let activeScanID else { return }
        do {
            _ = try send(method: "scan.cancel", params: ScanCancelParams(scanId: activeScanID))
            status = "Cancelling scan…"
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func reportExternalError(_ message: String) {
        errorMessage = message
        status = "Permission required"
    }

    private func statusForFailure(_ error: Error, duringScan: Bool) -> String {
        if let clientError = error as? EngineClientError {
            switch clientError {
            case .engineNotFound, .incompatibleEngine, .engineNotRunning:
                return "Engine unavailable"
            case .engineExited:
                return "Engine exited"
            case .incompleteStream, .engineRPC, .invalidMessage:
                return duringScan ? "Scan failed" : "Engine unavailable"
            }
        }
        return duringScan ? "Scan failed" : "Engine unavailable"
    }

    private func runSession(mode sessionMode: SessionMode, roots: [String]) async throws {
        stop()
        defer { stop() }
        mode = sessionMode
        scanRoots = roots
        nextID = 0
        helloRequestID = nil
        scanRequestID = nil
        assetsRequestID = nil
        portfolioRequestID = nil
        activeScanID = nil
        assetsLoaded = sessionMode == .probe
        portfolioLoaded = sessionMode == .probe

        let executable = try resolveEngineURL()
        enginePath = executable.path

        let process = Process()
        let stdinPipe = Pipe()
        let stdoutPipe = Pipe()
        process.executableURL = executable
        process.arguments = ["-database", inventoryDatabasePath()]
        process.standardInput = stdinPipe
        process.standardOutput = stdoutPipe
        // Inherit stderr so engine diagnostics appear in the `make run` terminal
        // and so a full stderr pipe cannot deadlock the child process.
        process.standardError = FileHandle.standardError
        process.currentDirectoryURL = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)

        do {
            try process.run()
        } catch {
            throw EngineClientError.invalidMessage(detail: "failed to launch \(executable.path): \(error.localizedDescription)")
        }
        self.process = process
        // Keep only the write end in the parent.
        stdinHandle = stdinPipe.fileHandleForWriting

        status = "Negotiating protocol…"
        helloRequestID = try send(method: "engine.hello", params: HelloParams())

        for try await line in stdoutPipe.fileHandleForReading.bytes.lines {
            try handle(line)
            if sessionMode == .probe, helloRequestID == nil {
                break
            }
            if sessionMode == .scan, assetsLoaded, portfolioLoaded {
                break
            }
            if !process.isRunning {
                if sessionMode == .scan, assetsLoaded, portfolioLoaded {
                    break
                }
                throw EngineClientError.engineExited(code: process.terminationStatus)
            }
        }

        if sessionMode == .probe {
            return
        }

        if progress.last?.complete != true {
            throw EngineClientError.incompleteStream
        }
    }

    private func handle(_ line: String) throws {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        let data = Data(trimmed.utf8)

        let header: RPCHeader
        do {
            header = try JSONDecoder().decode(RPCHeader.self, from: data)
        } catch {
            throw EngineClientError.invalidMessage(detail: "header decode failed for: \(trimmed.prefix(240))")
        }
        guard header.jsonrpc == "2.0" else {
            throw EngineClientError.invalidMessage(detail: "bad jsonrpc in \(trimmed.prefix(240))")
        }

        // Parse / server errors may omit id (JSON-RPC allows null id on parse failure).
        if let error = header.error {
            let matchesPending =
                header.id?.raw == helloRequestID ||
                header.id?.raw == scanRequestID ||
                header.id?.raw == assetsRequestID ||
                header.id?.raw == portfolioRequestID
            if header.id == nil || header.id?.isEmpty == true || matchesPending {
                throw EngineClientError.engineRPC(error)
            }
        }

        if header.id?.raw == helloRequestID {
            let hello = try JSONDecoder().decode(RPCEnvelope<HelloResult>.self, from: data)
            if let error = hello.error { throw EngineClientError.engineRPC(error) }
            guard let result = hello.result,
                  result.protocolVersion == protocolVersion,
                  result.schemaVersion == schemaVersion,
                  result.readOnly else { throw EngineClientError.incompatibleEngine }
            helloRequestID = nil
            if mode == .probe {
                status = "Engine ready"
                return
            }
            status = "Connected to read-only engine"
            scanRequestID = try send(method: "scan.start", params: ScanStartParams(
                roots: scanRoots, cancellationToken: UUID().uuidString
            ))
            return
        }
        if header.id?.raw == scanRequestID {
            let started = try JSONDecoder().decode(RPCEnvelope<ScanStarted>.self, from: data)
            if let error = started.error { throw EngineClientError.engineRPC(error) }
            guard let result = started.result else {
                throw EngineClientError.invalidMessage(detail: "scan.start missing result")
            }
            activeScanID = result.scanId
            status = "Scan running"
            return
        }
        if header.id?.raw == assetsRequestID {
            let envelope = try JSONDecoder().decode(RPCEnvelope<AssetsListResult>.self, from: data)
            if let error = envelope.error { throw EngineClientError.engineRPC(error) }
            guard let result = envelope.result else {
                throw EngineClientError.invalidMessage(detail: "assets.list missing result")
            }
            assets = result.assets
            relationships = result.relationships
            assetsLoaded = true
            status = "Loaded \(assets.count) assets"
            return
        }
        if header.id?.raw == portfolioRequestID {
            let envelope = try JSONDecoder().decode(RPCEnvelope<PortfolioListResult>.self, from: data)
            if let error = envelope.error { throw EngineClientError.engineRPC(error) }
            guard let result = envelope.result else {
                throw EngineClientError.invalidMessage(detail: "portfolio.list missing result")
            }
            portfolio = result.portfolio
            portfolioLoaded = true
            if assetsLoaded {
                status = "Scan complete · \(assets.count) assets"
            }
            return
        }
        guard header.id == nil, header.method == "scan.progress" else {
            throw EngineClientError.invalidMessage(detail: "unexpected message: \(trimmed.prefix(240))")
        }
        let event = try JSONDecoder().decode(ProgressEnvelope.self, from: data)
        guard event.method == "scan.progress", event.params.scanId == activeScanID else {
            throw EngineClientError.invalidMessage(detail: "progress for unknown scan")
        }
        progress.append(event.params)
        if event.params.complete == true {
            status = event.params.phase == "complete" ? "Scan complete" : "Scan \(event.params.phase)"
            if let scanID = activeScanID {
                assetsRequestID = try send(method: "assets.list", params: AssetsListParams(scanId: scanID))
                portfolioRequestID = try send(method: "portfolio.list", params: PortfolioListParams(scanId: scanID))
            } else {
                assetsLoaded = true
                portfolioLoaded = true
            }
        } else if event.params.phase == "detection", let found = event.params.assetsFound {
            status = "Detecting assets… \(found) found"
        } else {
            status = "Scanning: \(event.params.phase)"
        }
    }

    @discardableResult
    private func send<Params: Encodable>(method: String, params: Params) throws -> String {
        guard let stdinHandle else { throw EngineClientError.engineNotRunning }
        nextID += 1
        let id = String(nextID)

        // Encode params first, then build the envelope with JSONSerialization so the
        // wire format is always a single compact JSON object + newline.
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let paramsData = try encoder.encode(params)
        let paramsObject = try JSONSerialization.jsonObject(with: paramsData)

        let envelope: [String: Any] = [
            "jsonrpc": "2.0",
            "id": id,
            "method": method,
            "params": paramsObject,
        ]
        var data = try JSONSerialization.data(withJSONObject: envelope, options: [.sortedKeys])
        data.append(0x0A)
        try writeAll(data, to: stdinHandle)
        return id
    }

    private func writeAll(_ data: Data, to handle: FileHandle) throws {
        var offset = 0
        let fd = handle.fileDescriptor
        try data.withUnsafeBytes { rawBuffer in
            guard let base = rawBuffer.bindMemory(to: UInt8.self).baseAddress else { return }
            while offset < data.count {
                let written = Darwin.write(fd, base.advanced(by: offset), data.count - offset)
                if written < 0 {
                    throw EngineClientError.invalidMessage(detail: "stdin write failed: \(String(cString: strerror(errno)))")
                }
                if written == 0 {
                    throw EngineClientError.invalidMessage(detail: "stdin write returned 0 bytes")
                }
                offset += written
            }
        }
    }

    private func inventoryDatabasePath() -> String {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSTemporaryDirectory())
        let dir = base.appendingPathComponent("DevHearth", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("inventory.sqlite").path
    }

    private func resolveEngineURL() throws -> URL {
        var searched: [String] = []
        let fm = FileManager.default

        func consider(_ path: String) -> URL? {
            let url = URL(fileURLWithPath: path)
            searched.append(url.path)
            var isDir: ObjCBool = false
            guard fm.fileExists(atPath: url.path, isDirectory: &isDir), !isDir.boolValue else {
                return nil
            }
            return fm.isExecutableFile(atPath: url.path) ? url : nil
        }

        if let override = ProcessInfo.processInfo.environment["DEVHEARTH_ENGINE_PATH"], !override.isEmpty {
            if let url = consider(override) { return url }
        }

        if let bundled = Bundle.main.url(forAuxiliaryExecutable: "devhearth") {
            searched.append(bundled.path)
            if fm.isExecutableFile(atPath: bundled.path) { return bundled }
        }

        if let exe = Bundle.main.executableURL?.deletingLastPathComponent() {
            if let url = consider(exe.appendingPathComponent("devhearth").path) { return url }
        }
        let argv0 = CommandLine.arguments[0]
        let argvURL = URL(fileURLWithPath: argv0).deletingLastPathComponent().appendingPathComponent("devhearth")
        if let url = consider(argvURL.path) { return url }

        let cwd = fm.currentDirectoryPath
        let candidates = [
            "\(cwd)/build/devhearth",
            "\(cwd)/../build/devhearth",
            "\(cwd)/../../build/devhearth",
            "\(cwd)/devhearth",
        ]
        for path in candidates {
            if let url = consider((path as NSString).standardizingPath) { return url }
        }

        throw EngineClientError.engineNotFound(searched: searched)
    }
}
