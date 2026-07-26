import Foundation

/// Wire models for the Phase 4 policy, feedback, and trend methods.
///
/// `PolicyDocument` mirrors the engine's portable document field for field, and
/// it is the same shape the engine writes to an exported file. Nothing here can
/// hold an absolute path: roots travel as an alias plus a relative segment, and
/// the UI shows the alias form.

struct PolicyRoot: Codable, Hashable, Identifiable {
    var id: String { "\(alias)/\(relative ?? "")" }
    var alias: String
    var relative: String?

    /// Portable display form, matching the engine's own rendering.
    var display: String {
        guard let relative, !relative.isEmpty else { return "~" }
        return "~/\(relative)"
    }
}

struct PolicySuppression: Codable, Hashable, Identifiable {
    var id: String { "\(recommendationId ?? "")|\(family ?? "")|\(ecosystem ?? "")" }
    var recommendationId: String?
    var family: String?
    var ecosystem: String?
    var reason: String?
    var createdAt: String?

    var label: String {
        if let recommendationId, !recommendationId.isEmpty {
            return recommendationId
        }
        let family = familyLabel(family ?? "")
        guard let ecosystem, !ecosystem.isEmpty else { return family }
        return "\(family) · \(ecosystem)"
    }
}

struct PolicyRetention: Codable, Hashable {
    var scanHistoryCount: Int?
}

struct PolicyMachineSelector: Codable, Hashable {
    var architecture: String?
    var diskClass: String?
    var role: String?
}

struct PolicyOverlay: Codable, Hashable {
    var match: PolicyMachineSelector
    var preferredTools: [String: String]?
    var fitMode: String?
    var fitWeights: [String: Double]?
    var riskThreshold: String?
    var activeWithinDays: Int?
    var inactiveAfterDays: Int?
    var exclusions: [String]?
}

struct PolicyDocument: Codable, Hashable {
    var schemaVersion: Int
    var id: String
    var name: String?
    var roots: [PolicyRoot]?
    var exclusions: [String]?
    var preferredTools: [String: String]?
    var fitMode: String?
    var fitWeights: [String: Double]?
    var riskThreshold: String?
    var activeWithinDays: Int?
    var inactiveAfterDays: Int?
    var retention: PolicyRetention?
    var suppressions: [PolicySuppression]?
    var machineOverlays: [PolicyOverlay]?
}

struct PolicyMachine: Codable, Hashable {
    var architecture: String?
    var diskClass: String?
    var role: String?

    var summary: String {
        let parts = [architecture, diskClass, role].compactMap { value -> String? in
            guard let value, !value.isEmpty else { return nil }
            return value
        }
        return parts.isEmpty ? "unclassified machine" : parts.joined(separator: " · ")
    }
}

struct PolicyRootStatus: Codable, Hashable, Identifiable {
    var id: String { display }
    let display: String
    let alias: String
    let resolvable: Bool
}

/// The policy after machine overlays are applied: what the engine actually used.
struct EffectivePolicy: Codable, Hashable {
    let policyId: String
    let name: String?
    let fitMode: String
    let riskThreshold: String
    let preferredTools: [String: String]?
    let exclusions: [String]?
    let roots: [PolicyRootStatus]?
    let activeWithinDays: Int
    let inactiveAfterDays: Int
    let scanHistoryCount: Int
    let suppressionCount: Int
    let appliedOverlays: [PolicyMachine]?
}

/// Values the engine owns, so the UI never hardcodes a vocabulary.
struct PolicyChoices: Codable, Hashable {
    let fitModes: [String]
    let riskThresholds: [String]
    let verdicts: [String]
}

struct PolicyResult: Decodable {
    let document: PolicyDocument
    let effective: EffectivePolicy
    let machine: PolicyMachine
    let choices: PolicyChoices
    let warnings: [String]?
}

struct PolicyGetParams: Encodable {
    let policyId: String?
}

struct PolicySetParams: Encodable {
    let document: PolicyDocument
}

struct PolicyExportParams: Encodable {
    let policyId: String?
}

struct PolicyExportResult: Decodable {
    let document: String
    let schemaVersion: Int
    let suggestedName: String
    let notes: [String]?
}

struct PolicyImportParams: Encodable {
    let document: String
    let activate: Bool?
}

struct RecommendationsSuppressParams: Encodable {
    var recommendationId: String?
    var family: String?
    var ecosystem: String?
    var reason: String?
    var undo: Bool?
}

struct RecommendationsFeedbackParams: Encodable {
    var scanId: String?
    var recommendationId: String
    var family: String?
    var ecosystem: String?
    var verdict: String
    var note: String?
}

struct RecommendationsFeedbackResult: Decodable {
    let recommendationId: String
    let verdict: String
    let recordedAt: String
    let localOnly: Bool
}

struct TrendsListParams: Encodable {
    let limit: Int?
}

struct TrendPoint: Decodable, Hashable, Identifiable {
    var id: String { "\(scanId)-\(allocatedBytes)" }
    let scanId: String
    let at: String
    let allocatedBytes: Int64
}

struct TrendRegression: Decodable, Hashable, Identifiable {
    var id: String { "\(kind)|\(seriesKey ?? "")|\(scanId ?? "")" }
    let kind: String
    let severity: String
    let seriesKey: String?
    let label: String
    let detail: String
    let scanId: String?
    let deltaBytes: Int64?

    var isWarning: Bool { severity == "warning" }
}

struct TrendSeries: Decodable, Hashable, Identifiable {
    var id: String { key }
    let key: String
    let label: String
    let points: [TrendPoint]?
    let firstBytes: Int64
    let latestBytes: Int64
    let deltaBytes: Int64
    let percentChange: Double
    let bytesPerDay: Double
    let direction: String
    let uncertain: Bool?
    let regressions: [TrendRegression]?
}

struct TrendReport: Decodable {
    let snapshotCount: Int
    let from: String?
    let to: String?
    let comparedScope: String?
    let skippedSnapshots: Int?
    let total: TrendSeries
    let series: [TrendSeries]?
    let regressions: [TrendRegression]?
    let notes: [String]?
}

struct TrendsListResult: Decodable {
    let trends: TrendReport
}
