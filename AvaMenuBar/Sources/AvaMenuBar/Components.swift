import SwiftUI

/// A System-Settings-style row: a colored SF Symbol badge, a title (+ optional
/// subtitle), and trailing content. The shared shape every setting in this
/// app is built from, instead of each row hand-rolling its own HStack.
struct SettingsRow<Trailing: View>: View {
    let icon: String
    let tint: Color
    let title: String
    var subtitle: String?
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: icon)
                .font(.system(size: 11, weight: .semibold))
                .foregroundStyle(.white)
                .frame(width: 22, height: 22)
                .background(tint.gradient, in: RoundedRectangle(cornerRadius: 6, style: .continuous))

            // Fixed width (not maxWidth) so a long title/subtitle truncates
            // predictably instead of stealing space the trailing controls need —
            // without this, trailing content can get pushed out and clipped entirely.
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.system(size: 12, weight: .medium)).lineLimit(1)
                if let subtitle {
                    Text(subtitle)
                        .font(.system(size: 10).monospacedDigit())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .frame(width: 88, alignment: .leading)

            Spacer(minLength: 4)
            trailing
        }
        // The title lives in a sibling view from the control, so VoiceOver read
        // the two separately: "slider, 50 percent" with no way to tell which
        // slider. Combining them makes the row announce "Volume, 50 percent".
        // Ironic omission in the repo that ships `ava a11y`.
        .accessibilityElement(children: .combine)
        .accessibilityLabel(subtitle.map { "\(title), \($0)" } ?? title)
    }
}

/// Groups a stack of rows into a subtly-inset rounded card, native-settings-style.
struct SectionCard<Content: View>: View {
    var header: String?
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let header {
                Text(header)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(.secondary)
                    .textCase(.uppercase)
                    .padding(.leading, 4)
                    .padding(.bottom, 5)
            }
            VStack(spacing: 11) {
                content
            }
            .padding(10)
            .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        }
    }
}

/// A small colored status capsule, e.g. for the server's running/stopped state.
struct StatusBadge: View {
    let text: String
    let color: Color

    var body: some View {
        HStack(spacing: 5) {
            Circle().fill(color).frame(width: 6, height: 6)
            Text(text).font(.system(size: 11, weight: .medium))
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 3)
        .background(color.opacity(0.15), in: Capsule())
        .foregroundStyle(color)
    }
}
