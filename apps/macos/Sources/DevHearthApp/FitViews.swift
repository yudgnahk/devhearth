import SwiftUI

/// Per-ecosystem fit ranking with the factor scores that produced it.
struct FitView: View {
    let engine: EngineClient

    var body: some View {
        Group {
            if !engine.hasCompletedScan {
                ContentUnavailableView(
                    "No fit analysis yet",
                    systemImage: "slider.horizontal.3",
                    description: Text("Scan a developer folder to rank package-manager fit per ecosystem.")
                )
            } else if engine.fit.isEmpty {
                ContentUnavailableView(
                    "No ecosystems detected",
                    systemImage: "slider.horizontal.3",
                    description: Text("No ecosystem in this scan had enough evidence for fit analysis.")
                )
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        ForEach(engine.fit) { assessment in
                            FitAssessmentCard(assessment: assessment)
                        }
                    }
                    .padding(24)
                }
            }
        }
    }
}

struct FitAssessmentCard: View {
    let assessment: FitAssessment

    var body: some View {
        GroupBox {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Text(assessment.ecosystem.capitalized).font(.title3.bold())
                    Text(assessment.isDeep ? "deep analysis" : "shallow inventory")
                        .font(.caption2)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(.quaternary, in: Capsule())
                    Spacer()
                    Text("\(assessment.projectCount) projects")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                if let recommended = assessment.recommendedTool, !recommended.isEmpty {
                    Text(assessment.stayPutWins
                         ? "Recommended: keep \(recommended)"
                         : "Recommended: \(recommended) (baseline \(assessment.baseline ?? "none"))")
                        .font(.subheadline.weight(.semibold))
                }

                if let bytes = assessment.projectLocalInstallBytes, bytes > 0 {
                    Text("Project-local installs: \(formatBytes(bytes)) · shared stores: \(formatBytes(assessment.sharedStoreBytes ?? 0))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                ForEach(assessment.options ?? []) { option in
                    FitOptionRow(option: option)
                }

                ForEach(assessment.notes ?? [], id: \.self) { note in
                    Text("· \(note)").font(.caption2).foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

struct FitOptionRow: View {
    let option: FitOption
    @State private var expanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Button {
                expanded.toggle()
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: expanded ? "chevron.down" : "chevron.right")
                        .font(.caption2)
                    Text("#\(option.rank) \(option.tool)").font(.subheadline.weight(.semibold))
                    if option.stayPut {
                        Text("stay put")
                            .font(.caption2)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .background(.quaternary, in: Capsule())
                    }
                    if !option.installed {
                        Text("not installed").font(.caption2).foregroundStyle(.orange)
                    }
                    Spacer()
                    Text(String(format: "score %.2f", option.score))
                        .font(.caption)
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                }
            }
            .buttonStyle(.plain)

            if expanded {
                VStack(alignment: .leading, spacing: 4) {
                    if option.immediateSavingsHighBytes > 0 {
                        Text("Immediate savings \(formatBytes(option.immediateSavingsLowBytes))–\(formatBytes(option.immediateSavingsHighBytes))")
                            .font(.caption)
                            .monospacedDigit()
                    }
                    ForEach(option.factors ?? []) { factor in
                        HStack(alignment: .top, spacing: 6) {
                            Text(factor.kind.replacingOccurrences(of: "_", with: " "))
                                .font(.caption2.weight(.semibold))
                                .frame(width: 130, alignment: .leading)
                            ProgressView(value: factor.score)
                                .frame(width: 60)
                            Text(factor.detail)
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                    ForEach(option.blockers ?? [], id: \.self) { blocker in
                        Text("· \(blocker)").font(.caption2).foregroundStyle(.orange)
                    }
                    if let impact = option.workflowImpact, !impact.isEmpty {
                        Text(impact).font(.caption2).foregroundStyle(.secondary)
                    }
                }
                .padding(.leading, 18)
            }
        }
    }
}
