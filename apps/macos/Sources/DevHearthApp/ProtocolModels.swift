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
    let recommendationsFound: Int64?
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

/// Attributed storage for one asset. Exclusive bytes already subtract nested
/// assets, so project and node_modules figures can be shown side by side.
struct AssetSize: Decodable, Hashable {
    let attributed: Bool
    let logicalBytes: Int64
    let allocatedBytes: Int64
    let exclusiveAllocatedBytes: Int64
    let shared: Bool?
    let uncertain: Bool?
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
    let size: AssetSize?
    let lastActivityAt: String?

    // Wire field is "class"; Swift reserves that keyword.
    enum CodingKeys: String, CodingKey {
        case id, kind, displayName, path, risk, ecosystem
        case classification = "class"
        case detectorId, detectorVersion, evidence, size, lastActivityAt
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
    let projectLocalInstallCount: Int?
    let sharedStoreCount: Int?
    let downloadCacheCount: Int?
    let buildOutputCount: Int?
    let dominantPackageTool: String?
    let projectLocalInstallBytes: Int64?
    let sharedStoreBytes: Int64?
    let downloadCacheBytes: Int64?
    let buildOutputBytes: Int64?
    let sizesUncertain: Bool?
}

// MARK: - Phase 3 advice

struct FitListParams: Encodable { let scanId: String }

struct RecommendationsListParams: Encodable { let scanId: String }

/// One weighted signal behind a fit option, with the detail string the engine
/// produced so the UI never re-derives an explanation.
struct FitFactor: Decodable, Identifiable, Hashable {
    var id: String { kind }
    let kind: String
    let score: Double
    let weight: Double
    let detail: String
}

struct FitOption: Decodable, Identifiable, Hashable {
    var id: String { tool }
    let tool: String
    let stayPut: Bool
    let installed: Bool
    let projectsUsing: Int
    let rank: Int
    let score: Double
    let confidence: Double
    let factors: [FitFactor]?
    let dominantFactors: [String]?
    let blockers: [String]?
    let workflowImpact: String?
    let immediateSavingsLowBytes: Int64
    let immediateSavingsHighBytes: Int64
    let futureGrowthReductionBytes: Int64?
    let savingsUncertain: Bool?
}

struct FitAssessment: Decodable, Identifiable, Hashable {
    var id: String { ecosystem }
    let ecosystem: String
    let depth: String
    let projectCount: Int
    let baseline: String?
    let recommendedTool: String?
    let stayPutWins: Bool
    let options: [FitOption]?
    let notes: [String]?
    let projectLocalInstallBytes: Int64?
    let sharedStoreBytes: Int64?
    let versionManagers: [String]?

    var isDeep: Bool { depth == "deep" }
}

struct FitListResult: Decodable {
    let scanId: String
    let fit: [FitAssessment]
}

struct RecommendationSavings: Decodable, Hashable {
    let lowBytes: Int64
    let highBytes: Int64
    let futureGrowthReductionBytes: Int64?
    let uncertain: Bool?
}

struct RecommendationAlternative: Decodable, Identifiable, Hashable {
    var id: String { label }
    let label: String
    let stayPut: Bool?
    let rank: Int
    let score: Double
    let savingsLowBytes: Int64?
    let savingsHighBytes: Int64?
    let blockers: [String]?
}

struct AffectedAsset: Decodable, Identifiable, Hashable {
    let id: String
    let kind: String
    let displayName: String
    let path: String
}

struct RecommendationSummary: Decodable, Identifiable, Hashable {
    let id: String
    let family: String
    let title: String
    let ecosystem: String?
    let explanation: String
    let risk: String
    let confidence: Double
    let priority: Double
    let savings: RecommendationSavings
    let restorationCost: String?
    let compatibilityImpact: String?
    let preconditions: [String]?
    let proposedActions: [String]?
    let verification: [String]?
    let rollback: String?
    let blockers: [String]?
    let affectedAssets: [AffectedAsset]?
    let evidence: [EvidenceSummary]?
    let alternatives: [RecommendationAlternative]?
    let dominantFactors: [String]?
    let ruleId: String
    let ruleVersion: Int
    /// The engine restates that this protocol version cannot execute advice.
    let adviceOnly: Bool?

    var isBlocked: Bool { !(blockers ?? []).isEmpty }
}

struct RecommendationsListResult: Decodable {
    let scanId: String
    let recommendations: [RecommendationSummary]
}

/// Report-level roll-up of advice. Savings bounds stay separate on purpose.
struct AdviceSummary: Codable, Hashable {
    let recommendationCount: Int
    let byFamily: [String: Int]?
    let byRisk: [String: Int]?
    let blockedCount: Int
    let savingsLowBytes: Int64
    let savingsHighBytes: Int64
    let savingsUncertain: Bool?
    let fit: [FitHeadline]?
    let topRecommendationTitle: String?
}

struct FitHeadline: Codable, Hashable {
    let ecosystem: String
    let depth: String
    let baseline: String?
    let recommendedTool: String?
    let stayPutWins: Bool
    let confidence: Double?
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
    let advice: AdviceSummary?
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
