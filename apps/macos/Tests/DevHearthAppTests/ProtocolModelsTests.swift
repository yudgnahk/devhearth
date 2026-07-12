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
