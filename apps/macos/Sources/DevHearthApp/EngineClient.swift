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
    case noCompletedScan

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
        case .noCompletedScan:
            return "No completed scan is available. Scan a folder first."
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
    private(set) var directoryChildren: [DirectoryChild] = []
    private(set) var directoryPath = "" // redacted display path of current folder
    private(set) var directoryPathKey = "" // absolute key for inventory.children
    private(set) var directoryParentKey: String?
    private(set) var lastReport: ScanReport?
    private(set) var errorMessage: String?
    private(set) var isScanning = false
    private(set) var hasCompletedScan = false
    private(set) var enginePath: String?
    private(set) var activeScanID: String?

    private var process: Process?
    private var stdinHandle: FileHandle?
    private var nextID = 0
    private var pending: [String: CheckedContinuation<Data, Error>] = [:]
    private var scanCompleteWaiters: [CheckedContinuation<Void, Error>] = []
    private var readerTask: Task<Void, Never>?
    private var connectTask: Task<Void, Never>?

    /// Locate and hello-negotiate the engine, leaving the process running.
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
            try await ensureEngineRunning()
            let hello: HelloResult = try await request(method: "engine.hello", params: HelloParams())
            guard hello.protocolVersion == protocolVersion,
                  hello.schemaVersion == schemaVersion,
                  hello.readOnly else {
                throw EngineClientError.incompatibleEngine
            }
            status = "Engine ready · choose a folder to scan"
        } catch {
            errorMessage = error.localizedDescription
            status = statusForFailure(error, duringScan: false)
            stop()
        }
    }

    func start(roots: [String]) async {
        if let connectTask {
            await connectTask.value
        }
        do {
            progress = []
            assets = []
            relationships = []
            portfolio = []
            directoryChildren = []
            directoryPath = ""
            directoryPathKey = ""
            directoryParentKey = nil
            lastReport = nil
            hasCompletedScan = false
            activeScanID = nil
            errorMessage = nil
            isScanning = true
            status = "Starting scan…"

            try await ensureEngineRunning()
            let started: ScanStarted = try await request(
                method: "scan.start",
                params: ScanStartParams(roots: roots, cancellationToken: UUID().uuidString)
            )
            activeScanID = started.scanId
            status = "Scan running"
            try await waitForScanComplete()
            guard let scanID = activeScanID else {
                throw EngineClientError.incompleteStream
            }

            let listed: AssetsListResult = try await request(
                method: "assets.list",
                params: AssetsListParams(scanId: scanID)
            )
            assets = listed.assets
            relationships = listed.relationships

            let portfolioResult: PortfolioListResult = try await request(
                method: "portfolio.list",
                params: PortfolioListParams(scanId: scanID)
            )
            portfolio = portfolioResult.portfolio

            let report: ScanReport = try await request(
                method: "report.export",
                params: ReportExportParams(scanId: scanID)
            )
            lastReport = report

            try await loadChildren(pathKey: "")
            hasCompletedScan = true
            status = "Scan complete · \(assets.count) assets"
        } catch {
            errorMessage = error.localizedDescription
            status = statusForFailure(error, duringScan: true)
            hasCompletedScan = false
        }
        isScanning = false
    }

    func loadChildren(pathKey: String) async throws {
        guard let scanID = activeScanID else {
            throw EngineClientError.noCompletedScan
        }
        let result: InventoryChildrenResult = try await request(
            method: "inventory.children",
            params: InventoryChildrenParams(scanId: scanID, pathKey: pathKey.isEmpty ? nil : pathKey)
        )
        directoryChildren = result.children.sorted { lhs, rhs in
            if lhs.isDirectory != rhs.isDirectory {
                return lhs.isDirectory && !rhs.isDirectory
            }
            return lhs.name.localizedCaseInsensitiveCompare(rhs.name) == .orderedAscending
        }
        directoryPath = result.path
        directoryPathKey = result.pathKey ?? ""
        directoryParentKey = result.parentKey
    }

    func navigateInto(_ child: DirectoryChild) async {
        guard child.isDirectory else { return }
        do {
            try await loadChildren(pathKey: child.pathKey)
            status = "Browsing \(child.path)"
        } catch {
            errorMessage = error.localizedDescription
            status = "Browse failed"
        }
    }

    func navigateUp() async {
        guard let parent = directoryParentKey else {
            // Already at synthetic roots parent of selected roots.
            do {
                try await loadChildren(pathKey: "")
                status = "Browsing scan roots"
            } catch {
                errorMessage = error.localizedDescription
            }
            return
        }
        do {
            try await loadChildren(pathKey: parent)
            status = directoryPath.isEmpty ? "Browsing scan roots" : "Browsing \(directoryPath)"
        } catch {
            errorMessage = error.localizedDescription
            status = "Browse failed"
        }
    }

    /// Fetches a redacted JSON report (default path redaction on the engine).
    func exportReport() async throws -> ScanReport {
        guard let scanID = activeScanID else {
            throw EngineClientError.noCompletedScan
        }
        let report: ScanReport = try await request(
            method: "report.export",
            params: ReportExportParams(scanId: scanID)
        )
        lastReport = report
        return report
    }

    func stop() {
        for (_, cont) in pending {
            cont.resume(throwing: EngineClientError.engineNotRunning)
        }
        pending.removeAll()
        for waiter in scanCompleteWaiters {
            waiter.resume(throwing: EngineClientError.engineNotRunning)
        }
        scanCompleteWaiters.removeAll()
        readerTask?.cancel()
        readerTask = nil
        if let handle = stdinHandle {
            try? handle.close()
        }
        stdinHandle = nil
        if let process, process.isRunning {
            process.terminate()
            process.waitUntilExit()
        }
        process = nil
    }

    func cancelScan() {
        guard let activeScanID else { return }
        Task {
            do {
                struct CancelStatus: Decodable { let scanId: String; let status: String }
                let _: CancelStatus = try await request(
                    method: "scan.cancel",
                    params: ScanCancelParams(scanId: activeScanID)
                )
                status = "Cancelling scan…"
            } catch {
                errorMessage = error.localizedDescription
            }
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
            case .incompleteStream, .engineRPC, .invalidMessage, .noCompletedScan:
                return duringScan ? "Scan failed" : "Engine unavailable"
            }
        }
        return duringScan ? "Scan failed" : "Engine unavailable"
    }

    private func ensureEngineRunning() async throws {
        if let process, process.isRunning, stdinHandle != nil, readerTask != nil {
            return
        }
        stop()
        let executable = try resolveEngineURL()
        enginePath = executable.path

        let process = Process()
        let stdinPipe = Pipe()
        let stdoutPipe = Pipe()
        process.executableURL = executable
        process.arguments = ["-database", inventoryDatabasePath()]
        process.standardInput = stdinPipe
        process.standardOutput = stdoutPipe
        process.standardError = FileHandle.standardError
        process.currentDirectoryURL = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
        do {
            try process.run()
        } catch {
            throw EngineClientError.invalidMessage(detail: "failed to launch \(executable.path): \(error.localizedDescription)")
        }
        self.process = process
        stdinHandle = stdinPipe.fileHandleForWriting
        nextID = 0

        let handle = stdoutPipe.fileHandleForReading
        readerTask = Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                for try await line in handle.bytes.lines {
                    self.handleLine(line)
                }
            } catch {
                self.failAllPending(error)
            }
            self.handleReaderEOF()
        }
    }

    private func waitForScanComplete() async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            if let last = progress.last, last.complete == true {
                cont.resume()
                return
            }
            scanCompleteWaiters.append(cont)
        }
    }

    private func request<Params: Encodable, Result: Decodable>(method: String, params: Params) async throws -> Result {
        let id = try send(method: method, params: params)
        let data: Data = try await withCheckedThrowingContinuation { cont in
            pending[id] = cont
        }
        let envelope = try JSONDecoder().decode(RPCEnvelope<Result>.self, from: data)
        if let error = envelope.error {
            throw EngineClientError.engineRPC(error)
        }
        guard let result = envelope.result else {
            throw EngineClientError.invalidMessage(detail: "\(method) missing result")
        }
        return result
    }

    private func handleLine(_ line: String) {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        let data = Data(trimmed.utf8)

        let header: RPCHeader
        do {
            header = try JSONDecoder().decode(RPCHeader.self, from: data)
        } catch {
            failAllPending(EngineClientError.invalidMessage(detail: "header decode failed for: \(trimmed.prefix(240))"))
            return
        }
        guard header.jsonrpc == "2.0" else {
            failAllPending(EngineClientError.invalidMessage(detail: "bad jsonrpc"))
            return
        }

        if let id = header.id?.raw, !id.isEmpty, let cont = pending.removeValue(forKey: id) {
            if let error = header.error {
                cont.resume(throwing: EngineClientError.engineRPC(error))
            } else {
                cont.resume(returning: data)
            }
            return
        }

        if header.id == nil, header.method == "scan.progress" {
            do {
                let event = try JSONDecoder().decode(ProgressEnvelope.self, from: data)
                guard event.method == "scan.progress" else { return }
                if let activeScanID, event.params.scanId != activeScanID {
                    return
                }
                progress.append(event.params)
                if event.params.complete == true {
                    status = event.params.phase == "complete" ? "Scan complete" : "Scan \(event.params.phase)"
                    let waiters = scanCompleteWaiters
                    scanCompleteWaiters.removeAll()
                    for waiter in waiters {
                        waiter.resume()
                    }
                } else if event.params.phase == "detection", let found = event.params.assetsFound {
                    status = "Detecting assets… \(found) found"
                } else if event.params.phase == "persist" {
                    if let written = event.params.rowsWritten, let total = event.params.rowsTotal, total > 0 {
                        status = "Saving inventory… \(written)/\(total)"
                    } else {
                        status = "Saving inventory…"
                    }
                } else {
                    status = "Scanning: \(event.params.phase)"
                }
            } catch {
                errorMessage = error.localizedDescription
            }
            return
        }

        if let error = header.error {
            // Unmatched error without pending id.
            errorMessage = error.localizedDescription
        }
    }

    private func handleReaderEOF() {
        if let process, !process.isRunning {
            failAllPending(EngineClientError.engineExited(code: process.terminationStatus))
        } else {
            failAllPending(EngineClientError.incompleteStream)
        }
    }

    private func failAllPending(_ error: Error) {
        for (_, cont) in pending {
            cont.resume(throwing: error)
        }
        pending.removeAll()
        for waiter in scanCompleteWaiters {
            waiter.resume(throwing: error)
        }
        scanCompleteWaiters.removeAll()
    }

    @discardableResult
    private func send<Params: Encodable>(method: String, params: Params) throws -> String {
        guard let stdinHandle else { throw EngineClientError.engineNotRunning }
        nextID += 1
        let id = String(nextID)

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
