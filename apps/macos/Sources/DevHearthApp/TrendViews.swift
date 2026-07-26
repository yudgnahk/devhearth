import SwiftUI

/// Growth across repeat scans of the same roots.
///
/// Every figure here is a difference between two completed scans, so the view
/// leads with what a movement can and cannot mean. It never attributes a cause:
/// the engine can see that a series grew, not why.
struct TrendsView: View {
    let engine: EngineClient

    var body: some View {
        Group {
            if let report = engine.trends, report.snapshotCount > 0 {
                ScrollView {
                    VStack(alignment: .leading, spacing: 18) {
                        header(report)
                        if let regressions = report.regressions, !regressions.isEmpty {
                            regressionSection(regressions)
                        }
                        TrendSeriesCard(series: report.total, emphasized: true)
                        ForEach(report.series ?? []) { series in
                            TrendSeriesCard(series: series, emphasized: false)
                        }
                        if (report.series ?? []).isEmpty {
                            Text("No individual ecosystem or storage class was large enough to track separately.")
                                .appFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        notes(report)
                    }
                    .padding(24)
                }
            } else {
                ContentUnavailableView(
                    "No history yet",
                    systemImage: "chart.line.uptrend.xyaxis",
                    description: Text("Scan the same folders more than once and DevHearth will start reporting growth.")
                )
            }
        }
    }

    private func header(_ report: TrendReport) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Growth over \(report.snapshotCount) scans").appFont(.title2, weight: .bold)
            if let from = report.from, let to = report.to {
                Text("\(activityLabel(from)) → \(activityLabel(to))")
                    .appFont(.caption)
                    .foregroundStyle(.secondary)
            }
            if let skipped = report.skippedSnapshots, skipped > 0 {
                Label("\(skipped) earlier scan(s) covered different folders and are not compared",
                      systemImage: "exclamationmark.circle")
                    .appFont(.caption)
                    .foregroundStyle(.orange)
            }
        }
    }

    private func regressionSection(_ regressions: [TrendRegression]) -> some View {
        GroupBox("Changes worth a look") {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(regressions) { regression in
                    HStack(alignment: .top, spacing: 8) {
                        Image(systemName: regression.isWarning ? "exclamationmark.triangle.fill" : "info.circle")
                            .foregroundStyle(regression.isWarning ? .orange : .secondary)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(regression.detail).appFont(.callout)
                            Text(regression.kind.replacingOccurrences(of: "_", with: " "))
                                .appFont(.caption2)
                                .foregroundStyle(.tertiary)
                        }
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func notes(_ report: TrendReport) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(report.notes ?? [], id: \.self) { note in
                Text("· \(note)").appFont(.caption2).foregroundStyle(.secondary)
            }
        }
    }
}

struct TrendSeriesCard: View {
    let series: TrendSeries
    let emphasized: Bool

    var body: some View {
        GroupBox {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Text(series.label.capitalized)
                        .appFont(emphasized ? .title3 : .headline, weight: .semibold)
                    DirectionBadge(direction: series.direction)
                    Spacer()
                    Text(formatBytes(series.latestBytes))
                        .appFont(.headline)
                        .monospacedDigit()
                }

                Text(changeLine)
                    .appFont(.caption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)

                if let points = series.points, points.count > 1 {
                    Sparkline(points: points)
                        .frame(height: emphasized ? 56 : 36)
                }

                if series.uncertain == true {
                    Text("Includes shared or hard-linked storage, so these totals are lower bounds.")
                        .appFont(.caption2)
                        .foregroundStyle(.orange)
                }
                ForEach(series.regressions ?? []) { regression in
                    Text("· \(regression.detail)").appFont(.caption2).foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var changeLine: String {
        let delta = series.deltaBytes
        let sign = delta >= 0 ? "+" : "−"
        var line = "\(sign)\(formatBytes(abs(delta))) since the first compared scan"
        if series.percentChange != 0 {
            line += String(format: " (%.1f%%)", series.percentChange)
        }
        if series.bytesPerDay != 0 {
            let perDay = Int64(series.bytesPerDay.magnitude)
            line += " · \(series.bytesPerDay >= 0 ? "+" : "−")\(formatBytes(perDay))/day"
        }
        return line
    }
}

/// Minimal shape of a series over time. It is deliberately unlabelled: the
/// figures beside it are the record, and this only shows the shape.
struct Sparkline: View {
    let points: [TrendPoint]

    var body: some View {
        GeometryReader { geometry in
            let values = points.map { Double($0.allocatedBytes) }
            let lowest = values.min() ?? 0
            let highest = values.max() ?? 0
            let span = max(highest - lowest, 1)

            Path { path in
                for (index, value) in values.enumerated() {
                    let x = values.count > 1
                        ? geometry.size.width * Double(index) / Double(values.count - 1)
                        : 0
                    let y = geometry.size.height * (1 - (value - lowest) / span)
                    if index == 0 {
                        path.move(to: CGPoint(x: x, y: y))
                    } else {
                        path.addLine(to: CGPoint(x: x, y: y))
                    }
                }
            }
            .stroke(.tint, style: StrokeStyle(lineWidth: 2, lineJoin: .round))
        }
        .accessibilityLabel("Trend from \(formatBytes(points.first?.allocatedBytes ?? 0)) to \(formatBytes(points.last?.allocatedBytes ?? 0))")
    }
}

struct DirectionBadge: View {
    let direction: String

    var body: some View {
        Label(direction, systemImage: symbol)
            .appFont(.caption2, weight: .semibold)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }

    private var symbol: String {
        switch direction {
        case "growing": return "arrow.up.right"
        case "shrinking": return "arrow.down.right"
        default: return "equal"
        }
    }

    private var color: Color {
        switch direction {
        case "growing": return .orange
        case "shrinking": return .green
        default: return .secondary
        }
    }
}
