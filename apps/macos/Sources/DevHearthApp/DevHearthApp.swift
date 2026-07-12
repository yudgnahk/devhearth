import SwiftUI

@main
struct DevHearthApp: App {
    @State private var engine = EngineClient()

    var body: some Scene {
        WindowGroup("DevHearth") {
            ContentView(engine: engine)
                .frame(minWidth: 560, minHeight: 360)
                .task { await engine.start() }
                .onDisappear { engine.stop() }
        }
    }
}

struct ContentView: View {
    let engine: EngineClient

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("DevHearth").font(.largeTitle.bold())
            Text("Phase 0 protocol preview").foregroundStyle(.secondary)
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
    }
}
