/// Shared formatting helpers for advice, fit, and asset views. These are free
/// functions rather than view methods so their branching can be unit tested.
import Foundation

func formatBytes(_ value: Int64) -> String {
    ByteCountFormatter.string(fromByteCount: value, countStyle: .file)
}

func percent(_ value: Double) -> String {
    String(format: "%.0f%%", value * 100)
}

func savingsLabel(_ savings: RecommendationSavings) -> String {
    if savings.highBytes == 0 {
        return "No immediate savings"
    }
    if savings.lowBytes == savings.highBytes {
        return "Saves \(formatBytes(savings.lowBytes))"
    }
    return "Saves \(formatBytes(savings.lowBytes))–\(formatBytes(savings.highBytes))"
}

func familyLabel(_ family: String) -> String {
    family.replacingOccurrences(of: "_", with: " ")
}

/// Describes an asset's attributed storage: the subtree total, and when they
/// differ, the exclusive share that excludes nested assets such as a project's
/// own node_modules. Shared and uncertain totals are labelled rather than
/// presented as exact figures.
func sizeLine(_ size: AssetSize) -> String {
    var line = formatBytes(size.allocatedBytes)
    if size.exclusiveAllocatedBytes != size.allocatedBytes {
        line += " (\(formatBytes(size.exclusiveAllocatedBytes)) excluding nested assets)"
    }
    if size.shared == true {
        line += " · shared store"
    }
    if size.uncertain == true {
        line += " · lower bound (hard links present)"
    }
    return line
}

/// One-line portfolio summary for an ecosystem. Byte figures appear only when
/// attribution produced them, so a scan without sizes reads as counts alone.
func portfolioLine(_ item: PortfolioSummary) -> String {
    var parts = ["\(item.projectCount) projects"]
    if let tool = item.dominantPackageTool, !tool.isEmpty {
        parts.append("dominant \(tool)")
    }
    if let managers = item.versionManagers, !managers.isEmpty {
        parts.append("VM: \(managers.joined(separator: ", "))")
    }
    if let bytes = item.projectLocalInstallBytes, bytes > 0 {
        parts.append("local \(formatBytes(bytes))")
    }
    if let bytes = item.sharedStoreBytes, bytes > 0 {
        parts.append("stores \(formatBytes(bytes))")
    }
    return parts.joined(separator: " · ")
}

/// Renders the engine's RFC 3339 activity timestamp in the viewer's locale. An
/// unparseable value is shown verbatim rather than hidden: a timestamp the UI
/// cannot read is a protocol problem worth seeing.
func activityLabel(_ raw: String) -> String {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime]
    guard let date = formatter.date(from: raw) else {
        return raw
    }
    return date.formatted(date: .abbreviated, time: .shortened)
}
