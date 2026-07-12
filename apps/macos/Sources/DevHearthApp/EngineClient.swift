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
    private(set) var errorMessage: String?
    private(set) var isScanning = false

    private var process: Process?
    private var input: FileHandle?
    private var nextID = 0
	private var helloRequestID: String?
	private var scanRequestID: String?
	private var activeScanID: String?
	private var scanRoots: [String] = []

    func start(roots: [String]) async {
        do {
            progress = []
            errorMessage = nil
            isScanning = true
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
                if progress.last?.complete == true { break }
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
        } else {
            status = "Scanning: \(event.params.phase)"
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
