import Foundation
import Testing
@testable import DevHearthApp

@Test func decodesPolicyResultWithEffectiveOverlays() throws {
    let data = Data(#"""
    {
      "document": {
        "schemaVersion": 1,
        "id": "local-machine",
        "name": "Laptop",
        "roots": [{"alias": "home", "relative": "Projects"}],
        "fitMode": "prefer_disk_savings",
        "riskThreshold": "medium",
        "retention": {"scanHistoryCount": 30},
        "suppressions": [{"family": "adopt_shared_store", "reason": "not yet", "createdAt": "2026-07-01"}]
      },
      "effective": {
        "policyId": "local-machine",
        "name": "Laptop",
        "fitMode": "prefer_disk_savings",
        "riskThreshold": "medium",
        "roots": [{"display": "~/Projects", "alias": "home", "resolvable": true}],
        "activeWithinDays": 90,
        "inactiveAfterDays": 180,
        "scanHistoryCount": 30,
        "suppressionCount": 1,
        "appliedOverlays": [{"architecture": "arm64", "diskClass": "small"}]
      },
      "machine": {"architecture": "arm64", "diskClass": "small", "role": "laptop"},
      "choices": {
        "fitModes": ["balanced", "prefer_disk_savings", "prefer_workflow_stability"],
        "riskThresholds": ["informational", "low", "medium", "high"],
        "verdicts": ["accepted", "rejected", "unclear", "later"]
      }
    }
    """#.utf8)

    let result = try JSONDecoder().decode(PolicyResult.self, from: data)
    #expect(result.effective.fitMode == "prefer_disk_savings")
    #expect(result.effective.suppressionCount == 1)
    #expect(result.effective.roots?.first?.display == "~/Projects")
    #expect(result.effective.roots?.first?.resolvable == true)
    #expect(result.machine.summary == "arm64 · small · laptop")
    #expect(result.choices.verdicts.contains("rejected"))
    #expect(result.document.suppressions?.first?.label == "adopt shared store")
}

/// A policy the UI sends back must not carry an absolute path in any field; the
/// engine would reject it, and the point of the format is that it cannot happen.
@Test func encodedPolicyDocumentCarriesNoAbsolutePaths() throws {
    var document = PolicyDocument(schemaVersion: 1, id: "p1")
    document.name = "Laptop"
    document.roots = [PolicyRoot(alias: "home", relative: "Projects/work")]
    document.exclusions = ["node_modules"]
    document.fitMode = "balanced"

    let encoder = JSONEncoder()
    // Slash escaping is a JSON-encoding detail; turning it off keeps the
    // assertion about what the document carries rather than how it is spelled.
    encoder.outputFormatting = .withoutEscapingSlashes
    let encoded = try encoder.encode(PolicySetParams(document: document))
    let text = try #require(String(data: encoded, encoding: .utf8))
    #expect(!text.contains("/Users"))
    #expect(text.contains("Projects/work"))
}

/// Optional fields must be omitted rather than sent as null: the engine rejects
/// documents it cannot make sense of, and a null root list is one of those.
@Test func encodedPolicyDocumentOmitsUnsetFields() throws {
    let encoded = try JSONEncoder().encode(PolicyDocument(schemaVersion: 1, id: "p1"))
    let text = try #require(String(data: encoded, encoding: .utf8))
    #expect(!text.contains("null"))
    #expect(!text.contains("roots"))
}

@Test func policyRootDisplayFallsBackToHome() {
    #expect(PolicyRoot(alias: "home", relative: nil).display == "~")
    #expect(PolicyRoot(alias: "home", relative: "").display == "~")
    #expect(PolicyRoot(alias: "home", relative: "Code").display == "~/Code")
}

@Test func decodesTrendReportWithRegressions() throws {
    let data = Data(#"""
    {
      "trends": {
        "snapshotCount": 3,
        "from": "2026-06-01T12:00:00Z",
        "to": "2026-06-03T12:00:00Z",
        "comparedScope": "abc123",
        "skippedSnapshots": 1,
        "total": {
          "key": "total:allocated",
          "label": "All scanned storage",
          "points": [
            {"scanId": "s1", "at": "2026-06-01T12:00:00Z", "allocatedBytes": 100},
            {"scanId": "s2", "at": "2026-06-03T12:00:00Z", "allocatedBytes": 160}
          ],
          "firstBytes": 100,
          "latestBytes": 160,
          "deltaBytes": 60,
          "percentChange": 60,
          "bytesPerDay": 30,
          "direction": "growing"
        },
        "series": [],
        "regressions": [
          {
            "kind": "sudden_growth",
            "severity": "warning",
            "seriesKey": "ecosystem:node",
            "label": "node",
            "detail": "node grew by 4.0 GiB between the last two scans of these roots",
            "scanId": "s2",
            "deltaBytes": 4294967296
          }
        ],
        "notes": ["growth is measured between completed scans"]
      }
    }
    """#.utf8)

    let result = try JSONDecoder().decode(TrendsListResult.self, from: data)
    #expect(result.trends.snapshotCount == 3)
    #expect(result.trends.skippedSnapshots == 1)
    #expect(result.trends.total.direction == "growing")
    #expect(result.trends.total.points?.count == 2)
    #expect(result.trends.regressions?.first?.isWarning == true)
    #expect(result.trends.notes?.isEmpty == false)
}

@Test func decodesSuppressedRecommendationFlag() throws {
    let data = Data(#"""
    {
      "scanId": "scan_1",
      "recommendations": [
        {
          "id": "rec-1",
          "family": "adopt_shared_store",
          "title": "Share node dependencies",
          "explanation": "…",
          "risk": "medium",
          "confidence": 0.7,
          "priority": 1,
          "savings": {"lowBytes": 1024, "highBytes": 3276},
          "ruleId": "rule.shared_store",
          "ruleVersion": 1,
          "adviceOnly": true,
          "suppressed": true
        }
      ],
      "suppressedCount": 1,
      "hiddenByRiskCount": 2,
      "riskThreshold": "medium"
    }
    """#.utf8)

    let result = try JSONDecoder().decode(RecommendationsListResult.self, from: data)
    #expect(result.suppressedCount == 1)
    #expect(result.hiddenByRiskCount == 2)
    #expect(result.recommendations.first?.isSuppressed == true)
}

/// Advice with no `suppressed` field is live advice, not hidden advice.
@Test func recommendationWithoutSuppressedFieldIsVisible() throws {
    let data = Data(#"""
    {
      "scanId": "scan_1",
      "recommendations": [
        {
          "id": "rec-1",
          "family": "consolidate_runtime",
          "title": "Consolidate runtimes",
          "explanation": "…",
          "risk": "low",
          "confidence": 0.6,
          "priority": 1,
          "savings": {"lowBytes": 0, "highBytes": 0},
          "ruleId": "rule.runtime",
          "ruleVersion": 1
        }
      ]
    }
    """#.utf8)

    let result = try JSONDecoder().decode(RecommendationsListResult.self, from: data)
    #expect(result.recommendations.first?.isSuppressed == false)
    #expect(result.suppressedCount == nil)
}

@Test func scanStartOmitsPolicyIdSoTheEngineChoosesTheActivePolicy() throws {
    let params = ScanStartParams(roots: ["/tmp/x"], cancellationToken: "token")
    let text = try #require(String(data: JSONEncoder().encode(params), encoding: .utf8))
    #expect(!text.contains("policyId"))
    #expect(!text.contains("usePolicyRoots"))
}
