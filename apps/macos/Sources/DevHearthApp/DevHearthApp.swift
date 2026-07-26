import SwiftUI
import UniformTypeIdentifiers
import AppKit

@main
struct DevHearthApp: App {
    @State private var engine = EngineClient()

    var body: some Scene {
        WindowGroup("DevHearth") {
            ContentView(engine: engine)
                .frame(minWidth: 900, minHeight: 560)
                .task { await engine.connect() }
                .onDisappear { engine.stop() }
        }
    }
}

struct ContentView: View {
    let engine: EngineClient
    @State private var selectingFolder = false
    @State private var selectedAsset: AssetSummary?
    @State private var detailTab: DetailTab = .inventory
    @State private var exportError: String?

    private enum DetailTab: String, CaseIterable, Identifiable {
        case inventory = "Inventory"
        case assets = "Assets"
        case advice = "Advice"
        case fit = "Fit"
        var id: String { rawValue }
    }

    var body: some View {
        NavigationSplitView {
            VStack(alignment: .leading, spacing: 18) {
                Text("DevHearth").font(.largeTitle.bold())
                Text("Read-only inventory and asset graph").foregroundStyle(.secondary)
                HStack {
                    Button("Choose Folder to Scan…") { selectingFolder = true }
                        .disabled(engine.isScanning)
                    if engine.isScanning {
                        Button("Cancel", role: .cancel) { engine.cancelScan() }
                    }
                    Button("Retry Engine") {
                        Task { await engine.connect() }
                    }
                    .disabled(engine.isScanning)
                    Button("Export JSON…") {
                        Task { await exportJSON() }
                    }
                    .disabled(!engine.hasCompletedScan || engine.isScanning)
                }
                HStack {
                    ProgressView(value: engine.progress.last?.complete == true ? 1 : nil)
                    Text(engine.status)
                }
                if let path = engine.enginePath {
                    Text(path)
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                        .textSelection(.enabled)
                        .lineLimit(2)
                }
                if let error = engine.errorMessage {
                    Text(error)
                        .foregroundStyle(.red)
                        .textSelection(.enabled)
                        .font(.caption)
                }
                if let exportError {
                    Text(exportError)
                        .foregroundStyle(.red)
                        .font(.caption)
                }
                if let report = engine.lastReport {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Scan overview").font(.headline)
                        Text("\(report.entriesVisited) entries · \(ByteCountFormatter.string(fromByteCount: report.allocatedBytes, countStyle: .file)) allocated")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if let inaccessible = report.inaccessible, !inaccessible.isEmpty {
                            Text("\(inaccessible.count) inaccessible path(s)")
                                .font(.caption)
                                .foregroundStyle(.orange)
                        }
                        if let advice = report.advice, advice.recommendationCount > 0 {
                            Text("\(advice.recommendationCount) recommendation(s) · est. \(formatBytes(advice.savingsLowBytes))–\(formatBytes(advice.savingsHighBytes))")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            if advice.blockedCount > 0 {
                                Text("\(advice.blockedCount) blocked pending verification")
                                    .font(.caption2)
                                    .foregroundStyle(.orange)
                            }
                        }
                    }
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
            VStack(spacing: 0) {
                Picker("Detail", selection: $detailTab) {
                    ForEach(DetailTab.allCases) { tab in
                        Text(tab.rawValue).tag(tab)
                    }
                }
                .pickerStyle(.segmented)
                .padding()

                switch detailTab {
                case .inventory:
                    DirectoryBrowserView(engine: engine)
                case .assets:
                    if let asset = selectedAsset {
                        AssetDetailView(asset: asset, relationships: engine.relationships.filter {
                            $0.sourceId == asset.id || $0.targetId == asset.id
                        })
                    } else {
                        ContentUnavailableView(
                            "Select an asset",
                            systemImage: "point.3.connected.trianglepath.dotted",
                            description: Text("Scan a folder, then pick an asset from the sidebar.")
                        )
                    }
                case .advice:
                    AdviceView(engine: engine)
                case .fit:
                    FitView(engine: engine)
                }
            }
        }
        .fileImporter(isPresented: $selectingFolder, allowedContentTypes: [.folder]) { selection in
            switch selection {
            case .success(let url):
                let accessed = url.startAccessingSecurityScopedResource()
                if !accessed && isAppSandboxed() {
                    engine.reportExternalError("Could not access the selected folder. Grant folder access and try again.")
                    return
                }
                Task {
                    await engine.start(roots: [url.path])
                    detailTab = .inventory
                    if accessed {
                        url.stopAccessingSecurityScopedResource()
                    }
                }
            case .failure(let error):
                engine.reportExternalError(error.localizedDescription)
            }
        }
    }

    private func exportJSON() async {
        exportError = nil
        do {
            let report = try await engine.exportReport()
            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            let data = try encoder.encode(report)

            let panel = NSSavePanel()
            panel.allowedContentTypes = [.json]
            panel.nameFieldStringValue = "devhearth-report-\(report.scanId).json"
            panel.canCreateDirectories = true
            panel.title = "Export redacted scan report"
            panel.message = "Paths are redacted relative to selected roots by default."

            guard panel.runModal() == .OK, let url = panel.url else { return }
            try data.write(to: url, options: .atomic)
        } catch {
            exportError = error.localizedDescription
        }
    }

    /// True when the process is running inside App Sandbox (not bare SPM/`make run`).
    private func isAppSandboxed() -> Bool {
        ProcessInfo.processInfo.environment["APP_SANDBOX_CONTAINER_ID"] != nil
    }
}

struct DirectoryBrowserView: View {
    let engine: EngineClient

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Button {
                    Task { await engine.navigateUp() }
                } label: {
                    Label("Up", systemImage: "chevron.up")
                }
                // At the synthetic roots listing, pathKey is empty and there is no parent.
                .disabled(!engine.hasCompletedScan || (engine.directoryPathKey.isEmpty && engine.directoryParentKey == nil))

                Text(engine.directoryPath.isEmpty ? "Scan roots" : engine.directoryPath)
                    .font(.headline)
                    .lineLimit(2)
                    .textSelection(.enabled)
                Spacer()
                Text("\(engine.directoryChildren.count) items")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 10)

            Divider()

            if !engine.hasCompletedScan {
                ContentUnavailableView(
                    "No inventory yet",
                    systemImage: "folder",
                    description: Text("Scan a developer folder to drill into directory sizes.")
                )
            } else if engine.directoryChildren.isEmpty {
                ContentUnavailableView(
                    "Empty directory",
                    systemImage: "folder",
                    description: Text("No child entries under this path.")
                )
            } else {
                Table(engine.directoryChildren) {
                    TableColumn("Name") { (child: DirectoryChild) in
                        HStack(spacing: 6) {
                            Image(systemName: iconName(for: child))
                                .foregroundStyle(child.isDirectory ? .blue : .secondary)
                            if child.isDirectory {
                                Button(child.name) {
                                    Task { await engine.navigateInto(child) }
                                }
                                .buttonStyle(.plain)
                                .foregroundStyle(.primary)
                            } else {
                                Text(child.name)
                            }
                        }
                    }
                    .width(min: 160, ideal: 240)

                    TableColumn("Kind") { child in
                        Text(child.kind)
                            .foregroundStyle(.secondary)
                    }
                    .width(80)

                    TableColumn("Size") { child in
                        Text(ByteCountFormatter.string(fromByteCount: child.totalAllocatedBytes, countStyle: .file))
                            .monospacedDigit()
                    }
                    .width(100)

                    TableColumn("Logical") { child in
                        Text(ByteCountFormatter.string(fromByteCount: child.totalLogicalBytes, countStyle: .file))
                            .monospacedDigit()
                            .foregroundStyle(.secondary)
                    }
                    .width(100)

                    TableColumn("Children") { child in
                        Text(child.directChildCount.map(String.init) ?? "—")
                            .foregroundStyle(.secondary)
                    }
                    .width(70)

                    TableColumn("Path") { child in
                        Text(child.path)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .textSelection(.enabled)
                    }
                    .width(min: 160, ideal: 280)
                }
            }
        }
    }

    private func iconName(for child: DirectoryChild) -> String {
        if child.isSymlink == true { return "link" }
        if child.isDirectory { return "folder.fill" }
        return "doc"
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
                if let size = asset.size, size.attributed {
                    labeled("Allocated", sizeLine(size))
                }
                if let activity = asset.lastActivityAt, !activity.isEmpty {
                    labeled("Last source activity", activityLabel(activity))
                }

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
