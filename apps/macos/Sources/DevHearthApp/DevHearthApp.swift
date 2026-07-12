import SwiftUI
import UniformTypeIdentifiers

@main
struct DevHearthApp: App {
    @State private var engine = EngineClient()

    var body: some Scene {
        WindowGroup("DevHearth") {
            ContentView(engine: engine)
                .frame(minWidth: 720, minHeight: 480)
                .onDisappear { engine.stop() }
        }
    }
}

struct ContentView: View {
    let engine: EngineClient
    @State private var selectingFolder = false
    @State private var selectedAsset: AssetSummary?

    var body: some View {
        NavigationSplitView {
            VStack(alignment: .leading, spacing: 18) {
                Text("DevHearth").font(.largeTitle.bold())
                Text("Read-only development asset graph").foregroundStyle(.secondary)
                HStack {
                    Button("Choose Folder to Scan…") { selectingFolder = true }
                        .disabled(engine.isScanning)
                    if engine.isScanning {
                        Button("Cancel", role: .cancel) { engine.cancelScan() }
                    }
                }
                HStack {
                    ProgressView(value: engine.progress.last?.complete == true ? 1 : nil)
                    Text(engine.status)
                }
                if let error = engine.errorMessage {
                    Text(error).foregroundStyle(.red)
                }
                List(selection: $selectedAsset) {
                    if !engine.portfolio.isEmpty {
                        Section("Portfolio") {
                            ForEach(engine.portfolio) { item in
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(item.ecosystem.capitalized).font(.headline)
                                    Text(portfolioLine(item))
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }
                    }
                    Section("Assets") {
                        ForEach(engine.assets) { asset in
                            NavigationLink(value: asset) {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(asset.displayName)
                                    Text("\(asset.kind) · \(asset.detectorId)")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }
                    }
                }
            }
            .padding(16)
            .navigationSplitViewColumnWidth(min: 280, ideal: 340)
        } detail: {
            if let asset = selectedAsset {
                AssetDetailView(asset: asset, relationships: engine.relationships.filter {
                    $0.sourceId == asset.id || $0.targetId == asset.id
                })
            } else {
                ContentUnavailableView(
                    "Select an asset",
                    systemImage: "point.3.connected.trianglepath.dotted",
                    description: Text("Scan a folder to inspect detected projects, tools, stores, and evidence.")
                )
            }
        }
        .fileImporter(isPresented: $selectingFolder, allowedContentTypes: [.folder]) { selection in
            guard case .success(let url) = selection else { return }
            guard url.startAccessingSecurityScopedResource() else {
                return
            }
            Task {
                await engine.start(roots: [url.path])
                url.stopAccessingSecurityScopedResource()
            }
        }
    }

    private func portfolioLine(_ item: PortfolioSummary) -> String {
        var parts = ["\(item.projectCount) projects"]
        if let tool = item.dominantPackageTool, !tool.isEmpty {
            parts.append("dominant \(tool)")
        }
        if let managers = item.versionManagers, !managers.isEmpty {
            parts.append("VM: \(managers.joined(separator: ", "))")
        }
        return parts.joined(separator: " · ")
    }
}

struct AssetDetailView: View {
    let asset: AssetSummary
    let relationships: [RelationshipSummary]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text(asset.displayName).font(.title2.bold())
                labeled("Kind", asset.kind)
                labeled("Path", asset.path)
                labeled("Risk", asset.risk)
                if let ecosystem = asset.ecosystem, !ecosystem.isEmpty {
                    labeled("Ecosystem", ecosystem)
                }
                if let classification = asset.classification, !classification.isEmpty {
                    labeled("Class", classification)
                }
                labeled("Detector", "\(asset.detectorId) v\(asset.detectorVersion)")

                if let evidence = asset.evidence, !evidence.isEmpty {
                    Text("Evidence").font(.headline)
                    ForEach(evidence) { item in
                        VStack(alignment: .leading, spacing: 2) {
                            Text("\(item.kind) (\(String(format: "%.0f%%", item.confidence * 100)))")
                                .font(.subheadline.weight(.semibold))
                            Text(item.value)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .textSelection(.enabled)
                        }
                        .padding(.vertical, 2)
                    }
                }

                if !relationships.isEmpty {
                    Text("Relationships").font(.headline)
                    ForEach(relationships) { rel in
                        Text("\(rel.kind) · confidence \(String(format: "%.0f%%", rel.confidence * 100))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
    }

    private func labeled(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value).textSelection(.enabled)
        }
    }
}
