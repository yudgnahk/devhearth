import SwiftUI
import UniformTypeIdentifiers
import AppKit

/// Portable preferences: what to scan, how fit is weighted, what to hide.
///
/// Everything shown here is exportable and machine-independent. Scan roots
/// appear as `~/Projects` rather than as absolute paths, because the absolute
/// form is exactly what a shared policy must not carry.
struct PolicyView: View {
    let engine: EngineClient
    @State private var importing = false
    @State private var exportError: String?

    var body: some View {
        Group {
            if let result = engine.policy {
                ScrollView {
                    VStack(alignment: .leading, spacing: 18) {
                        header(result)
                        fitSection(result)
                        rootsSection(result)
                        suppressionsSection(result)
                        overlaySection(result)
                        portabilityNote
                    }
                    .padding(24)
                }
            } else {
                ContentUnavailableView(
                    "No policy loaded",
                    systemImage: "slider.horizontal.below.rectangle",
                    description: Text("The engine could not read a policy. Retry the engine connection to try again.")
                )
            }
        }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.json]) { selection in
            switch selection {
            case .success(let url):
                let accessed = url.startAccessingSecurityScopedResource()
                Task {
                    await engine.importPolicy(from: url)
                    if accessed { url.stopAccessingSecurityScopedResource() }
                }
            case .failure(let error):
                exportError = error.localizedDescription
            }
        }
    }

    private func header(_ result: PolicyResult) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(result.effective.name ?? "Local policy").appFont(.title2, weight: .bold)
                    Text("\(result.effective.policyId) · this Mac: \(result.machine.summary)")
                        .appFont(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Export…") { Task { await exportPolicy() } }
                Button("Import…") { importing = true }
            }
            if let exportError {
                Text(exportError).appFont(.caption).foregroundStyle(.red)
            }
            ForEach(result.warnings ?? [], id: \.self) { warning in
                Label(warning, systemImage: "exclamationmark.circle")
                    .appFont(.caption)
                    .foregroundStyle(.orange)
            }
        }
    }

    private func fitSection(_ result: PolicyResult) -> some View {
        GroupBox("Fit weighting") {
            VStack(alignment: .leading, spacing: 10) {
                Picker("Tradeoff", selection: fitModeBinding(result)) {
                    ForEach(result.choices.fitModes, id: \.self) { mode in
                        Text(familyLabel(mode)).tag(mode)
                    }
                }
                .pickerStyle(.radioGroup)

                Text("A mode changes how options are ranked. It never removes the stay-put baseline and never declares an absolute best tool.")
                    .appFont(.caption)
                    .foregroundStyle(.secondary)

                Divider()

                Picker("Show advice up to risk", selection: riskBinding(result)) {
                    ForEach(result.choices.riskThresholds, id: \.self) { risk in
                        Text(risk).tag(risk)
                    }
                }
                .pickerStyle(.menu)
                .frame(maxWidth: 320)

                Text("Hiding a risk class changes what the inbox shows. It never changes the risk the engine assigned.")
                    .appFont(.caption)
                    .foregroundStyle(.secondary)

                if let preferred = result.effective.preferredTools, !preferred.isEmpty {
                    Text("Preferred tools: " + preferred.sorted(by: { $0.key < $1.key }).map { "\($0.key) → \($0.value)" }.joined(separator: ", "))
                        .appFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func rootsSection(_ result: PolicyResult) -> some View {
        GroupBox("Scan roots") {
            VStack(alignment: .leading, spacing: 6) {
                if (result.effective.roots ?? []).isEmpty {
                    Text("No roots are stored in this policy. Choosing a folder to scan does not add one automatically.")
                        .appFont(.caption)
                        .foregroundStyle(.secondary)
                }
                ForEach(result.effective.roots ?? []) { root in
                    HStack(spacing: 6) {
                        Image(systemName: root.resolvable ? "folder" : "questionmark.folder")
                            .foregroundStyle(root.resolvable ? .blue : .orange)
                        Text(root.display).appFont(.callout)
                        if !root.resolvable {
                            Text("cannot be resolved on this Mac")
                                .appFont(.caption2)
                                .foregroundStyle(.orange)
                        }
                    }
                }
                if let exclusions = result.effective.exclusions, !exclusions.isEmpty {
                    Divider()
                    Text("Exclusions: " + exclusions.joined(separator: ", "))
                        .appFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func suppressionsSection(_ result: PolicyResult) -> some View {
        GroupBox("Hidden advice (\(result.effective.suppressionCount))") {
            VStack(alignment: .leading, spacing: 6) {
                if (result.document.suppressions ?? []).isEmpty {
                    Text("Nothing is hidden. Dismissing a recommendation records it here and travels with an export.")
                        .appFont(.caption)
                        .foregroundStyle(.secondary)
                }
                ForEach(result.document.suppressions ?? []) { suppression in
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 1) {
                            Text(suppression.label).appFont(.callout)
                            if let reason = suppression.reason, !reason.isEmpty {
                                Text(reason).appFont(.caption2).foregroundStyle(.secondary)
                            }
                            if let created = suppression.createdAt, !created.isEmpty {
                                Text("hidden since \(created)").appFont(.caption2).foregroundStyle(.tertiary)
                            }
                        }
                        Spacer()
                        Button("Restore") {
                            Task {
                                await engine.suppress(RecommendationsSuppressParams(
                                    recommendationId: suppression.recommendationId,
                                    family: suppression.family,
                                    ecosystem: suppression.ecosystem,
                                    undo: true
                                ))
                            }
                        }
                        .appFont(.caption)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder private func overlaySection(_ result: PolicyResult) -> some View {
        if let overlays = result.effective.appliedOverlays, !overlays.isEmpty {
            GroupBox("Machine overlays applied here") {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(overlays.enumerated()), id: \.offset) { _, overlay in
                        Text("· \(overlay.summary)").appFont(.caption)
                    }
                    Text("This Mac matched these overlays, so its weighting differs from the exported defaults.")
                        .appFont(.caption2)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    private var portabilityNote: some View {
        VStack(alignment: .leading, spacing: 3) {
            Label("An exported policy carries preferences only", systemImage: "lock.shield")
                .appFont(.caption, weight: .semibold)
            Text("No absolute paths, inventory, scan history, or recommendation feedback leave this Mac. Roots travel as aliases and resolve against the home directory of the machine that imports them — but a stored root keeps its folder names below the alias, so review the roots above before sharing.")
                .appFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    /// Edits go through the engine, which revalidates every field. The UI never
    /// stores a policy the engine has not accepted.
    private func fitModeBinding(_ result: PolicyResult) -> Binding<String> {
        Binding(
            get: { result.effective.fitMode },
            set: { mode in
                var document = result.document
                document.fitMode = mode
                Task { await engine.savePolicy(document) }
            }
        )
    }

    private func riskBinding(_ result: PolicyResult) -> Binding<String> {
        Binding(
            get: { result.effective.riskThreshold },
            set: { risk in
                var document = result.document
                document.riskThreshold = risk
                Task { await engine.savePolicy(document) }
            }
        )
    }

    private func exportPolicy() async {
        exportError = nil
        do {
            let result = try await engine.exportPolicy()
            let panel = NSSavePanel()
            panel.allowedContentTypes = [.json]
            panel.nameFieldStringValue = result.suggestedName
            panel.canCreateDirectories = true
            panel.title = "Export portable policy"
            // The engine owns the wording of what an export does and does not
            // contain; restating it here would let the two drift apart.
            panel.message = (result.notes ?? ["Preferences only; no absolute paths or inventory."]).joined(separator: " ")

            guard panel.runModal() == .OK, let url = panel.url else { return }
            // The engine's bytes are written verbatim: re-encoding here could
            // produce a file it would not accept back.
            try Data(result.document.utf8).write(to: url, options: .atomic)
        } catch {
            exportError = error.localizedDescription
        }
    }
}
