/// Shared formatting helpers for advice and fit views.
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
