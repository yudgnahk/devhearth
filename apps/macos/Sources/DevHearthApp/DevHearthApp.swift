import SwiftUI
import UniformTypeIdentifiers

@main
struct DevHearthApp: App {
    @State private var engine = EngineClient()

    var body: some Scene {
        WindowGroup("DevHearth") {
            ContentView(engine: engine)
                .frame(minWidth: 560, minHeight: 360)
                .onDisappear { engine.stop() }
        }
    }
}

struct ContentView: View {
    let engine: EngineClient
    @State private var selectingFolder = false

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("DevHearth").font(.largeTitle.bold())
            Text("Read-only storage inventory").foregroundStyle(.secondary)
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
            List(engine.progress, id: \.phase) { event in
                HStack {
                    Text(event.phase.capitalized)
                    Spacer()
                    Text("\(event.entriesVisited) entries")
                    Text(ByteCountFormatter.string(fromByteCount: event.allocatedBytes, countStyle: .file))
                        .foregroundStyle(.secondary)
                }
            }
            if let error = engine.errorMessage {
                Text(error).foregroundStyle(.red)
            }
        }
        .padding(24)
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
}
