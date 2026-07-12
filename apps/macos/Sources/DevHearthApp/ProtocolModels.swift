import Foundation

let protocolVersion = 1
let schemaVersion = 1

struct RPCRequest<Params: Encodable>: Encodable {
    let jsonrpc = "2.0"
    let id: String
    let method: String
    let params: Params
}

struct HelloParams: Encodable {
    let protocolVersions = [protocolVersion]
    let schemaVersions = [schemaVersion]
}

struct HelloResult: Decodable {
    let engine: String
    let protocolVersion: Int
    let schemaVersion: Int
    let readOnly: Bool
}

struct ScanStartParams: Encodable {
    let roots: [String]
    let policyId = "default"
    let cancellationToken: String
}

struct ScanStarted: Decodable { let scanId: String }

struct ScanProgress: Decodable {
    let scanId: String
    let phase: String
    let entriesVisited: Int64
    let allocatedBytes: Int64
    let complete: Bool?
}

struct RPCError: Decodable, LocalizedError {
    let code: Int
    let message: String

    var errorDescription: String? { "Engine error \(code): \(message)" }
}

struct RPCHeader: Decodable {
    let jsonrpc: String
    let id: String?
    let method: String?
}

struct RPCEnvelope<Result: Decodable>: Decodable {
    let jsonrpc: String
    let id: String?
    let result: Result?
    let error: RPCError?
}

struct ProgressEnvelope: Decodable {
    let jsonrpc: String
    let method: String
    let params: ScanProgress
}
