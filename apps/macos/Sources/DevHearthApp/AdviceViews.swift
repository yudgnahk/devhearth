import SwiftUI

/// Recommendation inbox: ranked advice on the left, full evidence on the right.
/// Nothing here can execute an operation; the engine only proposes plans.
struct AdviceView: View {
    let engine: EngineClient
    /// Selection is held as an id so the List highlight and the detail pane are
    /// driven by one value; holding the struct instead lets them disagree.
    @State private var selectedID: String?

    var body: some View {
        Group {
            if !engine.hasCompletedScan {
                ContentUnavailableView(
                    "No advice yet",
                    systemImage: "lightbulb",
                    description: Text("Scan a developer folder to see evidence-backed recommendations.")
                )
            } else if engine.recommendations.isEmpty {
                ContentUnavailableView(
                    "Nothing to optimize",
                    systemImage: "checkmark.seal",
                    description: Text("No structural optimization had enough evidence in this scan.")
                )
            } else {
                HSplitView {
                    VStack(alignment: .leading, spacing: 0) {
                        inbox
                        policyFooter
                    }
                    .frame(minWidth: 280, idealWidth: 340)
                    detail
                        .frame(minWidth: 360)
                }
                // Open on the highest-priority item, and recover if a new scan
                // retires the previously selected recommendation.
                .onAppear { selectFirstIfNeeded() }
                .onChange(of: engine.recommendations) { selectFirstIfNeeded() }
            }
        }
    }

    private var inbox: some View {
        List(engine.recommendations, selection: $selectedID) { item in
            VStack(alignment: .leading, spacing: 4) {
                Text(item.title).appFont(.headline)
                HStack(spacing: 6) {
                    RiskBadge(risk: item.risk)
                    Text(savingsLabel(item.savings))
                        .appFont(.caption)
                        .monospacedDigit()
                    if item.isBlocked {
                        Label("blocked", systemImage: "exclamationmark.triangle")
                            .appFont(.caption2)
                            .foregroundStyle(.orange)
                    }
                }
                Text("\(familyLabel(item.family)) · confidence \(percent(item.confidence))")
                    .appFont(.caption2)
                    .foregroundStyle(.secondary)
            }
            .padding(.vertical, 2)
            .tag(item.id)
        }
    }

    /// States what the policy withheld. An inbox that is simply shorter reads as
    /// a machine with less to fix, which would be a lie by omission.
    @ViewBuilder private var policyFooter: some View {
        let suppressed = engine.suppressedRecommendations.count
        let hidden = engine.hiddenByRiskCount
        if suppressed > 0 || hidden > 0 {
            Divider()
            VStack(alignment: .leading, spacing: 2) {
                if suppressed > 0 {
                    Text("\(suppressed) hidden by your policy — manage them in Policy")
                        .appFont(.caption2)
                        .foregroundStyle(.secondary)
                }
                if hidden > 0 {
                    Text("\(hidden) above your risk threshold")
                        .appFont(.caption2)
                        .foregroundStyle(.orange)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
    }

    @ViewBuilder private var detail: some View {
        if let item = engine.recommendations.first(where: { $0.id == selectedID }) {
            RecommendationDetailView(recommendation: item, engine: engine)
        } else {
            ContentUnavailableView(
                "Select a recommendation",
                systemImage: "list.bullet.rectangle",
                description: Text("Pick an item to see its evidence, blockers, and rollback plan.")
            )
        }
    }

    /// Selects the top-ranked recommendation unless a still-present one is chosen.
    private func selectFirstIfNeeded() {
        if let selectedID, engine.recommendations.contains(where: { $0.id == selectedID }) {
            return
        }
        selectedID = engine.recommendations.first?.id
    }
}

struct RecommendationDetailView: View {
    let recommendation: RecommendationSummary
    let engine: EngineClient
    @State private var note = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(recommendation.title).appFont(.title2, weight: .bold)
                    HStack(spacing: 8) {
                        RiskBadge(risk: recommendation.risk)
                        Text(familyLabel(recommendation.family))
                            .appFont(.caption)
                            .foregroundStyle(.secondary)
                        Text("confidence \(percent(recommendation.confidence))")
                            .appFont(.caption)
                            .foregroundStyle(.secondary)
                        if recommendation.isSuppressed {
                            Text("hidden by policy")
                                .appFont(.caption2, weight: .semibold)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(.quaternary, in: Capsule())
                        }
                    }
                }

                Text(recommendation.explanation).appFont(.body)

                decisionSection

                GroupBox("Estimated savings") {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(savingsLabel(recommendation.savings))
                            .appFont(.headline)
                            .monospacedDigit()
                        if let growth = recommendation.savings.futureGrowthReductionBytes, growth > 0 {
                            Text("Avoided future growth: \(formatBytes(growth))")
                                .appFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if recommendation.savings.uncertain == true {
                            Text("Estimates are ranges: hard links and missing content comparison make them lower bounds.")
                                .appFont(.caption)
                                .foregroundStyle(.orange)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }

                if let blockers = recommendation.blockers, !blockers.isEmpty {
                    bulletSection("Blockers", items: blockers, tint: .orange)
                }
                if let preconditions = recommendation.preconditions, !preconditions.isEmpty {
                    bulletSection("Preconditions", items: preconditions)
                }
                if let actions = recommendation.proposedActions, !actions.isEmpty {
                    bulletSection("Proposed plan (nothing runs automatically)", items: actions)
                }
                if let verification = recommendation.verification, !verification.isEmpty {
                    bulletSection("Verification", items: verification)
                }
                if let rollback = recommendation.rollback, !rollback.isEmpty {
                    labeled("Rollback", rollback)
                }
                if let cost = recommendation.restorationCost, !cost.isEmpty {
                    labeled("Restoration cost", cost)
                }
                if let impact = recommendation.compatibilityImpact, !impact.isEmpty {
                    labeled("Compatibility and workflow", impact)
                }

                if let alternatives = recommendation.alternatives, !alternatives.isEmpty {
                    GroupBox("Alternatives considered") {
                        VStack(alignment: .leading, spacing: 6) {
                            ForEach(alternatives) { alternative in
                                VStack(alignment: .leading, spacing: 2) {
                                    HStack(spacing: 6) {
                                        Text("#\(alternative.rank) \(alternative.label)")
                                            .appFont(.subheadline, weight: .semibold)
                                        if alternative.stayPut == true {
                                            Text("stay put")
                                                .appFont(.caption2)
                                                .padding(.horizontal, 5)
                                                .padding(.vertical, 1)
                                                .background(.quaternary, in: Capsule())
                                        }
                                        Text(String(format: "score %.2f", alternative.score))
                                            .appFont(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                    ForEach(alternative.blockers ?? [], id: \.self) { blocker in
                                        Text("· \(blocker)").appFont(.caption2).foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }

                if let affected = recommendation.affectedAssets, !affected.isEmpty {
                    GroupBox("Affected assets") {
                        VStack(alignment: .leading, spacing: 4) {
                            ForEach(affected) { asset in
                                VStack(alignment: .leading, spacing: 1) {
                                    Text("\(asset.displayName) · \(asset.kind)").appFont(.caption)
                                    Text(asset.path)
                                        .appFont(.caption2)
                                        .foregroundStyle(.secondary)
                                        .textSelection(.enabled)
                                }
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }

                if let evidence = recommendation.evidence, !evidence.isEmpty {
                    GroupBox("Evidence") {
                        VStack(alignment: .leading, spacing: 6) {
                            ForEach(evidence) { item in
                                VStack(alignment: .leading, spacing: 1) {
                                    Text("\(item.kind) (\(percent(item.confidence)))")
                                        .appFont(.caption, weight: .semibold)
                                    Text(item.value)
                                        .appFont(.caption2)
                                        .foregroundStyle(.secondary)
                                        .textSelection(.enabled)
                                }
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }

                VStack(alignment: .leading, spacing: 2) {
                    if recommendation.adviceOnly != false {
                        Label("Advice only — DevHearth cannot execute this", systemImage: "hand.raised")
                            .appFont(.caption2)
                            .foregroundStyle(.secondary)
                    }
                    Text("Rule \(recommendation.ruleId) v\(recommendation.ruleVersion)")
                        .appFont(.caption2)
                        .foregroundStyle(.tertiary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
    }

    /// Recording a decision and hiding advice are separate on purpose. A verdict
    /// is local feedback about whether the advice was useful; hiding it is a
    /// portable display preference. Neither performs the work.
    private var decisionSection: some View {
        GroupBox("Your decision") {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Button("Useful") { record(store: "accepted") }
                    Button("Not for me") { record(store: "rejected") }
                    Button("Unclear") { record(store: "unclear") }
                    Button("Later") { record(store: "later") }
                }
                TextField("Optional note (stays on this Mac)", text: $note)
                    .textFieldStyle(.roundedBorder)

                Divider()

                HStack(spacing: 8) {
                    if recommendation.isSuppressed {
                        Button("Show again") { suppress(undo: true, wholeFamily: false) }
                    } else {
                        Button("Hide this") { suppress(undo: false, wholeFamily: false) }
                        Button("Hide all \(familyLabel(recommendation.family))") {
                            suppress(undo: false, wholeFamily: true)
                        }
                    }
                }
                Text("Hiding is a display preference recorded in your portable policy. Verdicts stay on this Mac and are never exported.")
                    .appFont(.caption2)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func record(store verdict: String) {
        let trimmed = note.trimmingCharacters(in: .whitespacesAndNewlines)
        Task {
            await engine.sendFeedback(
                recommendation: recommendation,
                verdict: verdict,
                note: trimmed.isEmpty ? nil : trimmed
            )
        }
        note = ""
    }

    private func suppress(undo: Bool, wholeFamily: Bool) {
        let reason = note.trimmingCharacters(in: .whitespacesAndNewlines)
        Task {
            await engine.suppress(RecommendationsSuppressParams(
                recommendationId: wholeFamily ? nil : recommendation.id,
                family: wholeFamily ? recommendation.family : nil,
                ecosystem: wholeFamily ? recommendation.ecosystem : nil,
                reason: reason.isEmpty ? nil : reason,
                undo: undo
            ))
        }
        note = ""
    }

    private func bulletSection(_ title: String, items: [String], tint: Color = .primary) -> some View {
        GroupBox(title) {
            VStack(alignment: .leading, spacing: 4) {
                ForEach(items, id: \.self) { item in
                    Text("· \(item)").appFont(.caption).foregroundStyle(tint)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func labeled(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).appFont(.caption).foregroundStyle(.secondary)
            Text(value).appFont(.callout).textSelection(.enabled)
        }
    }
}

/// Risk class badge shared by the advice and fit views.
struct RiskBadge: View {
    let risk: String

    var body: some View {
        Text(risk)
            .appFont(.caption2, weight: .semibold)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }

    private var color: Color {
        switch risk {
        case "informational": return .blue
        case "low": return .green
        case "medium": return .orange
        case "high": return .red
        case "prohibited": return .purple
        default: return .secondary
        }
    }
}
