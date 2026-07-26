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
    let rowsWritten: Int64?
    let rowsTotal: Int64?
    let complete: Bool?
}

struct AssetsListParams: Encodable { let scanId: String }

struct PortfolioListParams: Encodable { let scanId: String }

struct ReportExportParams: Encodable { let scanId: String }

struct InventoryChildrenParams: Encodable {
    let scanId: String
    let pathKey: String?

    enum CodingKeys: String, CodingKey {
        case scanId, pathKey
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(scanId, forKey: .scanId)
        if let pathKey, !pathKey.isEmpty {
            try container.encode(pathKey, forKey: .pathKey)
        }
    }
}

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

struct PortfolioSummary: Codable, Identifiable, Hashable {
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

struct DirectoryChild: Decodable, Identifiable, Hashable {
    var id: String { pathKey }
    let name: String
    let path: String
    let pathKey: String
    let kind: String
    let logicalBytes: Int64
    let allocatedBytes: Int64
    let totalLogicalBytes: Int64
    let totalAllocatedBytes: Int64
    let directChildCount: Int?
    let isSymlink: Bool?

    var isDirectory: Bool { kind == "directory" }
}

struct InventoryChildrenResult: Decodable {
    let scanId: String
    let pathKey: String?
    let path: String
    let parentKey: String?
    let children: [DirectoryChild]
}

struct InaccessiblePath: Codable, Hashable {
    let path: String
    let reason: String
}

struct ScanReport: Codable {
    let scanId: String
    let status: String
    let roots: [String]
    let entriesVisited: Int64
    let logicalBytes: Int64
    let allocatedBytes: Int64
    let inaccessible: [InaccessiblePath]?
    let assetCount: Int?
    let assetsByKind: [String: Int]?
    let portfolio: [PortfolioSummary]?
}

struct RPCError: Decodable, LocalizedError {
    let code: Int
    let message: String

    var errorDescription: String? { "Engine error \(code): \(message)" }
}

/// JSON-RPC ids may be string or number on the wire.
struct RPCID: Decodable, Equatable, CustomStringConvertible {
    let raw: String

    init(_ raw: String) { self.raw = raw }

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let value = try? container.decode(String.self) {
            raw = value
            return
        }
        if let value = try? container.decode(Int.self) {
            raw = String(value)
            return
        }
        if container.decodeNil() {
            raw = ""
            return
        }
        throw DecodingError.typeMismatch(
            RPCID.self,
            .init(codingPath: decoder.codingPath, debugDescription: "RPC id must be string or number")
        )
    }

    var description: String { raw }
    var isEmpty: Bool { raw.isEmpty }
}

struct RPCHeader: Decodable {
    let jsonrpc: String
    let id: RPCID?
    let method: String?
    let error: RPCError?
}

struct RPCEnvelope<Result: Decodable>: Decodable {
    let jsonrpc: String
    let id: RPCID?
    let result: Result?
    let error: RPCError?
}

struct ProgressEnvelope: Decodable {
    let jsonrpc: String
    let method: String
    let params: ScanProgress
}
