import Foundation
import Observation

enum EngineClientError: LocalizedError {
    case engineNotFound
    case invalidMessage
    case incompatibleEngine
    case incompleteStream
    case engineNotRunning

    var errorDescription: String? {
        switch self {
        case .engineNotFound: "The bundled DevHearth engine could not be found."
        case .invalidMessage: "The engine returned an invalid protocol message."
        case .incompatibleEngine: "The engine protocol or schema is incompatible."
        case .incompleteStream: "The engine closed before the mock scan completed."
        case .engineNotRunning: "The engine is not running."
        }
    }
}

@MainActor
@Observable
final class EngineClient {
    private(set) var status = "Engine not started"
    private(set) var progress: [ScanProgress] = []
    private(set) var assets: [AssetSummary] = []
    private(set) var relationships: [RelationshipSummary] = []
    private(set) var portfolio: [PortfolioSummary] = []
    private(set) var errorMessage: String?
    private(set) var isScanning = false

    private var process: Process?
    private var input: FileHandle?
    private var nextID = 0
    private var helloRequestID: String?
    private var scanRequestID: String?
    private var assetsRequestID: String?
    private var portfolioRequestID: String?
    private var activeScanID: String?
    private var scanRoots: [String] = []
    private var assetsLoaded = false
    private var portfolioLoaded = false

    func start(roots: [String]) async {
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
            let executable = try engineURL()
            let process = Process()
            let stdinPipe = Pipe()
            let stdoutPipe = Pipe()
            process.executableURL = executable
            process.standardInput = stdinPipe
            process.standardOutput = stdoutPipe
            process.standardError = FileHandle.standardError
            try process.run()
            self.process = process
            input = stdinPipe.fileHandleForWriting

            status = "Negotiating protocol…"
            helloRequestID = try send(method: "engine.hello", params: HelloParams())
            for try await line in stdoutPipe.fileHandleForReading.bytes.lines {
                try handle(line)
                if assetsLoaded && portfolioLoaded { break }
            }
            guard progress.last?.complete == true else { throw EngineClientError.incompleteStream }
            stop()
        } catch {
            stop()
            errorMessage = error.localizedDescription
            status = "Engine unavailable"
        }
        isScanning = false
    }

    func stop() {
        input?.closeFile()
        input = nil
        if process?.isRunning == true {
            process?.terminate()
            process?.waitUntilExit()
        }
        process = nil
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

    private func handle(_ line: String) throws {
        let data = Data(line.utf8)
        let header = try JSONDecoder().decode(RPCHeader.self, from: data)
        guard header.jsonrpc == "2.0" else { throw EngineClientError.invalidMessage }

        if header.id == helloRequestID {
            let hello = try JSONDecoder().decode(RPCEnvelope<HelloResult>.self, from: data)
            if let error = hello.error { throw error }
            guard let result = hello.result,
                  result.protocolVersion == protocolVersion,
                  result.schemaVersion == schemaVersion,
                  result.readOnly else { throw EngineClientError.incompatibleEngine }
            status = "Connected to read-only engine"
            scanRequestID = try send(method: "scan.start", params: ScanStartParams(
                roots: scanRoots, cancellationToken: UUID().uuidString
            ))
            return
        }
        if header.id == scanRequestID {
            let started = try JSONDecoder().decode(RPCEnvelope<ScanStarted>.self, from: data)
            if let error = started.error { throw error }
            guard let result = started.result else { throw EngineClientError.invalidMessage }
            activeScanID = result.scanId
            status = "Scan running"
            return
        }
        if header.id == assetsRequestID {
            let envelope = try JSONDecoder().decode(RPCEnvelope<AssetsListResult>.self, from: data)
            if let error = envelope.error { throw error }
            guard let result = envelope.result else { throw EngineClientError.invalidMessage }
            assets = result.assets
            relationships = result.relationships
            assetsLoaded = true
            status = "Loaded \(assets.count) assets"
            return
        }
        if header.id == portfolioRequestID {
            let envelope = try JSONDecoder().decode(RPCEnvelope<PortfolioListResult>.self, from: data)
            if let error = envelope.error { throw error }
            guard let result = envelope.result else { throw EngineClientError.invalidMessage }
            portfolio = result.portfolio
            portfolioLoaded = true
            if assetsLoaded {
                status = "Scan complete · \(assets.count) assets"
            }
            return
        }
        guard header.id == nil, header.method == "scan.progress" else {
            throw EngineClientError.invalidMessage
        }
        let event = try JSONDecoder().decode(ProgressEnvelope.self, from: data)
        guard event.method == "scan.progress", event.params.scanId == activeScanID else {
            throw EngineClientError.invalidMessage
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
        } else {
            if event.params.phase == "detection", let found = event.params.assetsFound {
                status = "Detecting assets… \(found) found"
            } else {
                status = "Scanning: \(event.params.phase)"
            }
        }
    }

    @discardableResult
    private func send<Params: Encodable>(method: String, params: Params) throws -> String {
        guard let input else { throw EngineClientError.engineNotRunning }
        nextID += 1
        let id = String(nextID)
        let request = RPCRequest(id: id, method: method, params: params)
        var data = try JSONEncoder().encode(request)
        data.append(0x0A)
        try input.write(contentsOf: data)
        return id
    }

    private func engineURL() throws -> URL {
        if let override = ProcessInfo.processInfo.environment["DEVHEARTH_ENGINE_PATH"], !override.isEmpty {
            return URL(fileURLWithPath: override)
        }
        if let bundled = Bundle.main.url(forAuxiliaryExecutable: "devhearth") { return bundled }
        throw EngineClientError.engineNotFound
    }
}
