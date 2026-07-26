import Foundation
import Testing
@testable import DevHearthApp

@Test func textScaleClampsToTheUsableRange() {
    #expect(TextScale.clamp(0.1) == TextScale.minimum)
    #expect(TextScale.clamp(9) == TextScale.maximum)
    #expect(TextScale.clamp(1.2) == 1.2)
}

/// The stored preference is a file a user can edit, so a nonsense value must not
/// be able to render the window unreadable.
@Test func textScaleRejectsNonFiniteValues() {
    #expect(TextScale.clamp(.nan) == TextScale.default)
    #expect(TextScale.clamp(.infinity) == TextScale.maximum)
    #expect(TextScale.clamp(-.infinity) == TextScale.minimum)
}

@Test func textScaleStepsStayInRange() {
    var value = TextScale.default
    for _ in 0..<50 { value = TextScale.larger(than: value) }
    #expect(value == TextScale.maximum)

    for _ in 0..<50 { value = TextScale.smaller(than: value) }
    #expect(value == TextScale.minimum)
}

@Test func textScaleStepsAreReversible() {
    let bigger = TextScale.larger(than: 1.0)
    #expect(bigger > 1.0)
    #expect(TextScale.smaller(than: bigger) == 1.0)
}

@Test func textScaleLabelReadsAsAPercentage() {
    #expect(TextScale.label(1.0) == "100%")
    #expect(TextScale.label(1.5) == "150%")
    #expect(TextScale.label(0.8) == "80%")
}

/// Scaling has to reach every role, or zooming leaves part of the window behind.
@Test func everyFontRoleScalesWithTheMultiplier() {
    let roles: [AppFont] = [.largeTitle, .title2, .title3, .headline, .body, .callout, .subheadline, .caption, .caption2]
    for role in roles {
        #expect(role.baseSize > 0)
        #expect(role.resolve(scale: 2) != role.resolve(scale: 1))
    }
}

/// The base ramp exists because macOS defaults are too small on a large display;
/// body text must stay above the 13pt system default.
@Test func baseRampIsLargerThanTheSystemDefault() {
    #expect(AppFont.body.baseSize >= 14)
    #expect(AppFont.caption2.baseSize >= 11)
    #expect(AppFont.largeTitle.baseSize > AppFont.title2.baseSize)
    #expect(AppFont.caption.baseSize > AppFont.caption2.baseSize)
}
