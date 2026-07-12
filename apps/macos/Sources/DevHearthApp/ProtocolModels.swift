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

struct ScanCancelParams: Encodable { let scanId: String }

struct ScanProgress: Decodable {
    let scanId: String
    let phase: String
    let entriesVisited: Int64
    let allocatedBytes: Int64
    let assetsFound: Int64?
    let complete: Bool?
}

struct AssetsListParams: Encodable { let scanId: String }

struct PortfolioListParams: Encodable { let scanId: String }

struct EvidenceSummary: Decodable, Identifiable, Hashable {
    var id: String { "\(kind):\(value):\(confidence)" }
    let kind: String
    let value: String
    let confidence: Double
}

struct AssetSummary: Decodable, Identifiable, Hashable {
    let id: String
    let kind: String
    let displayName: String
    let path: String
    let risk: String
    let ecosystem: String?
    let classification: String?
    let detectorId: String
    let detectorVersion: Int
    let evidence: [EvidenceSummary]?

    // Wire field is "class"; Swift reserves that keyword.
    enum CodingKeys: String, CodingKey {
        case id, kind, displayName, path, risk, ecosystem
        case classification = "class"
        case detectorId, detectorVersion, evidence
    }
}

struct RelationshipSummary: Decodable, Identifiable, Hashable {
    let id: String
    let sourceId: String
    let targetId: String
    let kind: String
    let confidence: Double
    let detectorId: String?
}

struct AssetsListResult: Decodable {
    let scanId: String
    let assets: [AssetSummary]
    let relationships: [RelationshipSummary]
}

struct PortfolioSummary: Decodable, Identifiable, Hashable {
    var id: String { ecosystem }
    let ecosystem: String
    let projectCount: Int
    let packageManagers: [String: Int]?
    let versionManagers: [String]?
    let sharedStoreCount: Int?
    let downloadCacheCount: Int?
    let buildOutputCount: Int?
    let dominantPackageTool: String?
}

struct PortfolioListResult: Decodable {
    let scanId: String
    let portfolio: [PortfolioSummary]
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
