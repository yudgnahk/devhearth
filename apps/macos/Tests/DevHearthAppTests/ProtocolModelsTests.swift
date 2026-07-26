import Foundation
import Testing
@testable import DevHearthApp

@Test func decodesProgressEvent() throws {
    let data = Data(#"{"jsonrpc":"2.0","method":"scan.progress","params":{"scanId":"mock","phase":"metadata","entriesVisited":12,"allocatedBytes":4096,"complete":false}}"#.utf8)
    let event = try JSONDecoder().decode(ProgressEnvelope.self, from: data)
    #expect(event.method == "scan.progress")
    #expect(event.params.entriesVisited == 12)
}

@Test func helloRequestMatchesGoWireFormat() throws {
    let request = RPCRequest(id: "hello", method: "engine.hello", params: HelloParams())
    let object = try #require(JSONSerialization.jsonObject(with: JSONEncoder().encode(request)) as? [String: Any])
    #expect(object["jsonrpc"] as? String == "2.0")
    #expect(object["method"] as? String == "engine.hello")
    let params = try #require(object["params"] as? [String: Any])
    #expect(params["protocolVersions"] as? [Int] == [1])
    #expect(params["schemaVersions"] as? [Int] == [1])
}

@Test func rpcErrorSurfacesEngineMessage() {
    let error = RPCError(code: -32001, message: "incompatible")
    #expect(error.localizedDescription.contains("incompatible"))
}

@Test func decodesAssetsListResult() throws {
    let data = Data(#"""
    {"scanId":"scan_01","assets":[{"id":"a1","kind":"project","displayName":"app","path":"<selected-root-1>/app","risk":"informational","ecosystem":"node","class":"project","detectorId":"detect.node","detectorVersion":1,"evidence":[{"kind":"manifest","value":"package.json","confidence":0.9}]}],"relationships":[]}
    """#.utf8)
    let result = try JSONDecoder().decode(AssetsListResult.self, from: data)
    #expect(result.assets.count == 1)
    #expect(result.assets[0].classification == "project")
    #expect(result.assets[0].evidence?.first?.kind == "manifest")
}

@Test func decodesInventoryChildrenAndRoundTripsReport() throws {
    let childrenData = Data(#"""
    {"scanId":"scan_01","pathKey":"/Users/me/Projects","path":"<selected-root-1>","parentKey":"","children":[{"name":"app","path":"<selected-root-1>/app","pathKey":"/Users/me/Projects/app","kind":"directory","logicalBytes":0,"allocatedBytes":0,"totalLogicalBytes":100,"totalAllocatedBytes":4096,"directChildCount":2}]}
    """#.utf8)
    let children = try JSONDecoder().decode(InventoryChildrenResult.self, from: childrenData)
    #expect(children.children.count == 1)
    #expect(children.children[0].isDirectory)
    #expect(children.children[0].totalAllocatedBytes == 4096)

    let reportData = Data(#"""
    {"scanId":"scan_01","status":"complete","roots":["<selected-root-1>"],"entriesVisited":12,"logicalBytes":100,"allocatedBytes":4096,"inaccessible":[],"assetCount":1,"assetsByKind":{"project":1},"portfolio":[{"ecosystem":"node","projectCount":1}]}
    """#.utf8)
    let report = try JSONDecoder().decode(ScanReport.self, from: reportData)
    let encoded = try JSONEncoder().encode(report)
    let roundTrip = try JSONDecoder().decode(ScanReport.self, from: encoded)
    #expect(roundTrip.scanId == "scan_01")
    #expect(roundTrip.roots == ["<selected-root-1>"])
}

@Test func engineSubprocessNegotiatesAndCompletesMockScan() throws {
    guard let path = ProcessInfo.processInfo.environment["DEVHEARTH_ENGINE_PATH"] else { return }
    let process = Process()
    let input = Pipe()
    let output = Pipe()
    process.executableURL = URL(fileURLWithPath: path)
    process.arguments = ["--mock-scan"]
    process.standardInput = input
    process.standardOutput = output
    try process.run()

    let messages = [
        #"{"jsonrpc":"2.0","id":"1","method":"engine.hello","params":{"protocolVersions":[1],"schemaVersions":[1]}}"#,
        #"{"jsonrpc":"2.0","id":"2","method":"scan.start","params":{"roots":["/synthetic"]}}"#,
    ].joined(separator: "\n") + "\n"
    try input.fileHandleForWriting.write(contentsOf: Data(messages.utf8))
    input.fileHandleForWriting.closeFile()
    let response = String(decoding: output.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
    process.waitUntilExit()

    #expect(process.terminationStatus == 0)
    #expect(response.contains(#""protocolVersion":1"#))
    #expect(response.contains(#""complete":true"#))
}

@Test func decodesRecommendationInboxWithSafetyFields() throws {
    let data = Data(#"""
    {"scanId":"scan_01","recommendations":[{"id":"rec_000000000001","family":"adopt_shared_store","title":"Share node dependencies","ecosystem":"node","explanation":"two installs could share one store","risk":"medium","confidence":0.7,"priority":1000,"savings":{"lowBytes":1024,"highBytes":2048,"futureGrowthReductionBytes":1536,"uncertain":true},"restorationCost":"one install per project","compatibilityImpact":"moderate","preconditions":["every project has a lockfile"],"proposedActions":["migrate one project first"],"verification":["each project builds"],"rollback":"reinstall with the previous tool","blockers":["native addons unverified"],"affectedAssets":[{"id":"install-1","kind":"project_local_install","displayName":"node_modules","path":"<selected-root-1>/app/node_modules"}],"evidence":[{"kind":"path_signature","value":"<selected-root-1>/app/node_modules","confidence":0.95}],"alternatives":[{"label":"npm","stayPut":true,"rank":2,"score":0.62}],"dominantFactors":["in_use_share"],"ruleId":"rule.adopt_shared_store","ruleVersion":1}]}
    """#.utf8)
    let result = try JSONDecoder().decode(RecommendationsListResult.self, from: data)
    let recommendation = try #require(result.recommendations.first)
    #expect(recommendation.risk == "medium")
    #expect(recommendation.savings.highBytes == 2048)
    #expect(recommendation.savings.uncertain == true)
    #expect(recommendation.isBlocked)
    #expect(recommendation.alternatives?.first?.stayPut == true)
    #expect(recommendation.affectedAssets?.first?.path == "<selected-root-1>/app/node_modules")
}

@Test func decodesFitAssessmentWithFactors() throws {
    let data = Data(#"""
    {"scanId":"scan_01","fit":[{"ecosystem":"node","depth":"deep","projectCount":2,"baseline":"npm","recommendedTool":"pnpm","stayPutWins":false,"options":[{"tool":"pnpm","stayPut":false,"installed":true,"projectsUsing":0,"rank":1,"score":0.7,"confidence":0.7,"factors":[{"kind":"in_use_share","score":0.5,"weight":0.3,"detail":"1 of 2 projects"}],"dominantFactors":["in_use_share"],"immediateSavingsLowBytes":1024,"immediateSavingsHighBytes":2048},{"tool":"npm","stayPut":true,"installed":true,"projectsUsing":2,"rank":2,"score":0.6,"confidence":0.8,"immediateSavingsLowBytes":0,"immediateSavingsHighBytes":0}],"notes":["no content hashing has run"]}]}
    """#.utf8)
    let result = try JSONDecoder().decode(FitListResult.self, from: data)
    let assessment = try #require(result.fit.first)
    #expect(assessment.isDeep)
    #expect(assessment.options?.count == 2)
    #expect(assessment.options?.first?.factors?.first?.kind == "in_use_share")
    #expect(assessment.options?.last?.stayPut == true)
    #expect(assessment.notes?.isEmpty == false)
}

@Test func decodesAssetSizeAndActivity() throws {
    let data = Data(#"""
    {"scanId":"scan_01","assets":[{"id":"a1","kind":"project","displayName":"app","path":"<selected-root-1>/app","risk":"informational","detectorId":"detect.node","detectorVersion":1,"size":{"attributed":true,"logicalBytes":10,"allocatedBytes":8192,"exclusiveAllocatedBytes":4096,"uncertain":true},"lastActivityAt":"2026-07-01T12:00:00Z"}],"relationships":[]}
    """#.utf8)
    let result = try JSONDecoder().decode(AssetsListResult.self, from: data)
    let asset = try #require(result.assets.first)
    #expect(asset.size?.attributed == true)
    #expect(asset.size?.exclusiveAllocatedBytes == 4096)
    #expect(asset.size?.uncertain == true)
    #expect(asset.lastActivityAt == "2026-07-01T12:00:00Z")
}

@Test func reportRoundTripsAdviceSummary() throws {
    let data = Data(#"""
    {"scanId":"scan_01","status":"complete","roots":["<selected-root-1>"],"entriesVisited":12,"logicalBytes":100,"allocatedBytes":4096,"inaccessible":[],"assetCount":2,"advice":{"recommendationCount":2,"byFamily":{"adopt_shared_store":1,"hibernate_inactive_project":1},"byRisk":{"medium":1,"low":1},"blockedCount":1,"savingsLowBytes":1024,"savingsHighBytes":4096,"savingsUncertain":true,"fit":[{"ecosystem":"node","depth":"deep","baseline":"npm","recommendedTool":"pnpm","stayPutWins":false,"confidence":0.7}],"topRecommendationTitle":"Share node dependencies"}}
    """#.utf8)
    let report = try JSONDecoder().decode(ScanReport.self, from: data)
    let roundTrip = try JSONDecoder().decode(ScanReport.self, from: try JSONEncoder().encode(report))
    #expect(roundTrip.advice?.recommendationCount == 2)
    #expect(roundTrip.advice?.blockedCount == 1)
    #expect(roundTrip.advice?.savingsHighBytes == 4096)
    #expect(roundTrip.advice?.fit?.first?.recommendedTool == "pnpm")
}

@Test func savingsLabelCollapsesEqualBounds() {
    let equal = RecommendationSavings(lowBytes: 2048, highBytes: 2048, futureGrowthReductionBytes: nil, uncertain: nil)
    let range = RecommendationSavings(lowBytes: 1024, highBytes: 4096, futureGrowthReductionBytes: nil, uncertain: nil)
    let none = RecommendationSavings(lowBytes: 0, highBytes: 0, futureGrowthReductionBytes: nil, uncertain: nil)
    #expect(!savingsLabel(equal).contains("–"))
    #expect(savingsLabel(range).contains("–"))
    #expect(savingsLabel(none) == "No immediate savings")
}
