import SwiftUI

/// Recommendation inbox: ranked advice on the left, full evidence on the right.
/// Nothing here can execute an operation; the engine only proposes plans.
struct AdviceView: View {
    let engine: EngineClient
    @State private var selected: RecommendationSummary?

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
                    inbox
                        .frame(minWidth: 280, idealWidth: 340)
                    detail
                        .frame(minWidth: 360)
                }
            }
        }
    }

    private var inbox: some View {
        List(engine.recommendations, selection: $selected) { item in
            Button {
                selected = item
            } label: {
                VStack(alignment: .leading, spacing: 4) {
                    Text(item.title).font(.headline)
                    HStack(spacing: 6) {
                        RiskBadge(risk: item.risk)
                        Text(savingsLabel(item.savings))
                            .font(.caption)
                            .monospacedDigit()
                        if item.isBlocked {
                            Label("blocked", systemImage: "exclamationmark.triangle")
                                .font(.caption2)
                                .foregroundStyle(.orange)
                        }
                    }
                    Text("\(familyLabel(item.family)) · confidence \(percent(item.confidence))")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                .padding(.vertical, 2)
            }
            .buttonStyle(.plain)
        }
    }

    @ViewBuilder private var detail: some View {
        if let item = selected ?? engine.recommendations.first {
            RecommendationDetailView(recommendation: item)
        } else {
            ContentUnavailableView(
                "Select a recommendation",
                systemImage: "list.bullet.rectangle",
                description: Text("Pick an item to see its evidence, blockers, and rollback plan.")
            )
        }
    }
}

struct RecommendationDetailView: View {
    let recommendation: RecommendationSummary

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(recommendation.title).font(.title2.bold())
                    HStack(spacing: 8) {
                        RiskBadge(risk: recommendation.risk)
                        Text(familyLabel(recommendation.family))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Text("confidence \(percent(recommendation.confidence))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }

                Text(recommendation.explanation).font(.body)

                GroupBox("Estimated savings") {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(savingsLabel(recommendation.savings))
                            .font(.headline)
                            .monospacedDigit()
                        if let growth = recommendation.savings.futureGrowthReductionBytes, growth > 0 {
                            Text("Avoided future growth: \(formatBytes(growth))")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if recommendation.savings.uncertain == true {
                            Text("Estimates are ranges: hard links and missing content comparison make them lower bounds.")
                                .font(.caption)
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
                                            .font(.subheadline.weight(.semibold))
                                        if alternative.stayPut == true {
                                            Text("stay put")
                                                .font(.caption2)
                                                .padding(.horizontal, 5)
                                                .padding(.vertical, 1)
                                                .background(.quaternary, in: Capsule())
                                        }
                                        Text(String(format: "score %.2f", alternative.score))
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                    ForEach(alternative.blockers ?? [], id: \.self) { blocker in
                                        Text("· \(blocker)").font(.caption2).foregroundStyle(.secondary)
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
                                    Text("\(asset.displayName) · \(asset.kind)").font(.caption)
                                    Text(asset.path)
                                        .font(.caption2)
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
                                        .font(.caption.weight(.semibold))
                                    Text(item.value)
                                        .font(.caption2)
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
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                    Text("Rule \(recommendation.ruleId) v\(recommendation.ruleVersion)")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(24)
        }
    }

    private func bulletSection(_ title: String, items: [String], tint: Color = .primary) -> some View {
        GroupBox(title) {
            VStack(alignment: .leading, spacing: 4) {
                ForEach(items, id: \.self) { item in
                    Text("· \(item)").font(.caption).foregroundStyle(tint)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func labeled(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.callout).textSelection(.enabled)
        }
    }
}

/// Risk class badge shared by the advice and fit views.
struct RiskBadge: View {
    let risk: String

    var body: some View {
        Text(risk)
            .font(.caption2.weight(.semibold))
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
