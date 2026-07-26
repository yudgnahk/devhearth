import SwiftUI

/// App-owned text scale.
///
/// macOS has no Dynamic Type, so the system text styles resolve to fixed point
/// sizes and `⌘+` does nothing in a plain SwiftUI window. Every view therefore
/// asks for a role (`.appFont(.caption)`) instead of a fixed style, and this
/// file resolves that role against the user's chosen scale.
///
/// The base ramp is deliberately larger than the AppKit defaults: the densest
/// views here are evidence tables read on large external displays, where 10pt
/// caption text is unreadable at a normal viewing distance.
enum AppFont {
    case largeTitle
    case title2
    case title3
    case headline
    case body
    case callout
    case subheadline
    case caption
    case caption2

    /// Point size at scale 1.0.
    var baseSize: CGFloat {
        switch self {
        case .largeTitle: return 30
        case .title2: return 20
        case .title3: return 17
        case .headline: return 15
        case .body: return 14
        case .callout: return 13
        case .subheadline: return 13
        case .caption: return 12
        case .caption2: return 11
        }
    }

    /// Weight used when a call site does not ask for one.
    var baseWeight: Font.Weight {
        switch self {
        case .largeTitle, .title2, .title3, .headline: return .semibold
        default: return .regular
        }
    }

    func resolve(scale: CGFloat, weight: Font.Weight? = nil) -> Font {
        .system(size: baseSize * scale, weight: weight ?? baseWeight)
    }
}

/// Bounds and step for the text scale. The range is wide enough to cover a
/// 27-inch display at arm's length without letting a stray shortcut shrink the
/// UI into unreadability.
enum TextScale {
    static let minimum = 0.8
    static let maximum = 2.0
    static let step = 0.1
    static let `default` = 1.0

    /// Clamps an arbitrary stored or user-supplied value into the usable range.
    /// Storage is untrusted here too: a hand-edited preference must not be able
    /// to render the window unusable.
    static func clamp(_ value: Double) -> Double {
        // Only NaN needs special handling: it is unordered, so min/max cannot
        // bring it back into range. Infinities clamp to the bounds like any
        // other out-of-range number.
        guard !value.isNaN else { return `default` }
        return min(maximum, max(minimum, (value * 100).rounded() / 100))
    }

    static func larger(than value: Double) -> Double { clamp(clamp(value) + step) }

    static func smaller(than value: Double) -> Double { clamp(clamp(value) - step) }

    /// "120%" for menus and the sidebar readout.
    static func label(_ value: Double) -> String {
        "\(Int((clamp(value) * 100).rounded()))%"
    }
}

private struct AppTextScaleKey: EnvironmentKey {
    static let defaultValue: CGFloat = CGFloat(TextScale.default)
}

extension EnvironmentValues {
    /// Multiplier applied to every `.appFont` role in the view tree.
    var appTextScale: CGFloat {
        get { self[AppTextScaleKey.self] }
        set { self[AppTextScaleKey.self] = newValue }
    }
}

private struct AppFontModifier: ViewModifier {
    @Environment(\.appTextScale) private var scale
    let role: AppFont
    let weight: Font.Weight?

    func body(content: Content) -> some View {
        content.font(role.resolve(scale: scale, weight: weight))
    }
}

extension View {
    /// Applies a scaled text role. Prefer this over `.font(...)` so the whole
    /// app responds to the zoom commands.
    func appFont(_ role: AppFont, weight: Font.Weight? = nil) -> some View {
        modifier(AppFontModifier(role: role, weight: weight))
    }
}

/// Sidebar control mirroring the View-menu zoom commands. The menu shortcuts
/// are the fast path; this exists because a shortcut nobody can find is not a
/// fix for text being too small.
struct TextScaleControl: View {
    @Binding var scale: Double

    var body: some View {
        HStack(spacing: 6) {
            Button {
                scale = TextScale.smaller(than: scale)
            } label: {
                Image(systemName: "textformat.size.smaller")
            }
            .disabled(scale <= TextScale.minimum)
            .help("Smaller text (⌘−)")

            Text(TextScale.label(scale))
                .appFont(.caption2)
                .monospacedDigit()
                .foregroundStyle(.secondary)
                .frame(width: 44)

            Button {
                scale = TextScale.larger(than: scale)
            } label: {
                Image(systemName: "textformat.size.larger")
            }
            .disabled(scale >= TextScale.maximum)
            .help("Bigger text (⌘+)")

            Button("Reset") { scale = TextScale.default }
                .appFont(.caption2)
                .disabled(scale == TextScale.default)
                .help("Actual size (⌘0)")
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Text size")
    }
}
