import SwiftUI

// ─────────────────────────────────────────────
// MARK: - Flag + country helpers
// ─────────────────────────────────────────────
private let countryFlags: [String: String] = [
    "us": "🇺🇸", "gb": "🇬🇧", "de": "🇩🇪", "fr": "🇫🇷", "nl": "🇳🇱",
    "se": "🇸🇪", "jp": "🇯🇵", "sg": "🇸🇬", "au": "🇦🇺", "ca": "🇨🇦",
    "br": "🇧🇷", "in": "🇮🇳", "ae": "🇦🇪", "za": "🇿🇦", "ch": "🇨🇭",
    "no": "🇳🇴", "dk": "🇩🇰", "fi": "🇫🇮", "es": "🇪🇸", "it": "🇮🇹",
]
private let countryTaglines: [String: String] = [
    "us": "Ultra-fast • Tier-1 backbone",
    "gb": "Low-latency • EU border",
    "de": "Privacy-first • Central EU",
    "fr": "Premium • Paris datacenter",
    "nl": "Peering hub • AMS-IX",
    "se": "GDPR-strong • Nordic privacy",
    "jp": "Asia gateway • Low-latency",
    "sg": "Asia-Pacific hub",
    "au": "Oceania • Sydney core",
    "ca": "North America • No 5-eyes risk",
    "br": "South America • São Paulo",
    "in": "South Asia • Mumbai core",
    "ae": "Middle East • Dubai hub",
    "za": "Africa • Johannesburg",
]
private func flag(_ code: String) -> String { countryFlags[code.lowercased()] ?? "🌐" }
private func tagline(_ code: String) -> String { countryTaglines[code.lowercased()] ?? "VPN server" }

// ─────────────────────────────────────────────
// MARK: - Root view
// ─────────────────────────────────────────────
struct ContentView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        ZStack {
            // Subtle gradient background
            LinearGradient(
                colors: [Color(nsColor: .windowBackgroundColor), Theme.accent.opacity(0.04)],
                startPoint: .top, endPoint: .bottom
            ).ignoresSafeArea()

            if !model.isEnrolled {
                EnrollView()
            } else {
                MainView()
            }
        }
        .frame(minWidth: 520, minHeight: 700)
        .onAppear { if model.isEnrolled { model.loadLocations() } }
    }
}

// ─────────────────────────────────────────────
// MARK: - Enroll view
// ─────────────────────────────────────────────
private struct EnrollView: View {
    @EnvironmentObject var model: AppModel
    @State private var invite = ""
    @State private var email  = ""

    var body: some View {
        VStack(spacing: 0) {
            // Hero header
            VStack(spacing: 8) {
                Text("🔐").font(.system(size: 52))
                HStack(spacing: 6) {
                    Text("Aevora").font(.system(size: 34, weight: .bold))
                    Circle().fill(Theme.accent).frame(width: 10, height: 10)
                        .offset(y: -10)
                }
                Text("Private • Secure • Yours")
                    .font(.subheadline).foregroundStyle(.secondary)
            }
            .padding(.top, 60).padding(.bottom, 40)

            // Card
            VStack(alignment: .leading, spacing: 16) {
                Text("Activate this device").font(.title3.weight(.semibold))
                    .padding(.bottom, 4)

                VStack(alignment: .leading, spacing: 6) {
                    Label("Invite code", systemImage: "ticket")
                        .font(.caption.weight(.medium)).foregroundStyle(.secondary)
                    TextField("Paste invite code…", text: $invite)
                        .textFieldStyle(.roundedBorder)
                        .font(.system(.body, design: .monospaced))
                }

                VStack(alignment: .leading, spacing: 6) {
                    Label("Email", systemImage: "envelope")
                        .font(.caption.weight(.medium)).foregroundStyle(.secondary)
                    TextField("you@example.com", text: $email)
                        .textFieldStyle(.roundedBorder)
                }

                Button(action: {
                    model.enroll(invite: invite, email: email, deviceName: Platform.deviceName)
                }) {
                    HStack {
                        if model.phase == .connecting {
                            ProgressView().controlSize(.small)
                        }
                        Text("Activate →")
                            .font(.headline)
                    }
                    .frame(maxWidth: .infinity).frame(height: 44)
                }
                .buttonStyle(.borderedProminent).tint(Theme.accent)
                .disabled(invite.isEmpty || email.isEmpty)

                if let err = model.lastError {
                    Label(err, systemImage: "exclamationmark.triangle")
                        .font(.caption).foregroundStyle(.red)
                }
            }
            .padding(28)
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 20))
            .shadow(color: .black.opacity(0.08), radius: 20, y: 8)
            .frame(maxWidth: 380)

            Spacer()
        }
        .padding()
    }
}

// ─────────────────────────────────────────────
// MARK: - Main (post-enroll) view
// ─────────────────────────────────────────────
private struct MainView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 0) {
            // ── Top bar ──────────────────────────────
            HStack {
                HStack(spacing: 6) {
                    Text("Aevora").font(.system(size: 22, weight: .bold))
                    Circle().fill(Theme.accent).frame(width: 8, height: 8)
                        .offset(y: -8)
                }
                Spacer()
                StatusPill()
            }
            .padding(.horizontal, 20).padding(.top, 16).padding(.bottom, 12)

            Divider().opacity(0.5)

            ScrollView {
                VStack(spacing: 16) {
                    // Map
                    WorldMapView()
                        .frame(height: 200)
                        .clipShape(RoundedRectangle(cornerRadius: 16))
                        .padding(.horizontal, 16).padding(.top, 16)

                    // Connect card
                    ConnectCard()
                        .padding(.horizontal, 16)

                    // Country grid
                    CountryGrid()
                        .padding(.horizontal, 16).padding(.bottom, 20)
                }
            }
        }
    }
}

// ─────────────────────────────────────────────
// MARK: - Status pill
// ─────────────────────────────────────────────
private struct StatusPill: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(pillColor)
                .frame(width: 8, height: 8)
                .overlay(
                    model.phase == .connected ?
                        Circle().stroke(pillColor.opacity(0.4), lineWidth: 3)
                            .scaleEffect(1.4)
                        : nil
                )
            Text(pillText)
                .font(.caption.weight(.semibold))
                .foregroundStyle(pillColor)
        }
        .padding(.horizontal, 10).padding(.vertical, 5)
        .background(pillColor.opacity(0.12), in: Capsule())
    }

    private var pillText: String {
        switch model.phase {
        case .connected: return "CONNECTED"
        case .connecting: return "CONNECTING…"
        case .failed: return "FAILED"
        default: return "DISCONNECTED"
        }
    }
    private var pillColor: Color {
        switch model.phase {
        case .connected: return .green
        case .connecting: return Theme.accent
        case .failed: return .red
        default: return .secondary
        }
    }
}

// ─────────────────────────────────────────────
// MARK: - Connect card
// ─────────────────────────────────────────────
private struct ConnectCard: View {
    @EnvironmentObject var model: AppModel

    private var selectedLoc: FfiLocation? {
        guard let code = model.selectedCountry else { return nil }
        return model.locations.first(where: { $0.code == code })
    }

    var body: some View {
        VStack(spacing: 14) {
            // Selected country hero
            if let loc = selectedLoc {
                HStack(spacing: 12) {
                    Text(flag(loc.code))
                        .font(.system(size: 44))

                    VStack(alignment: .leading, spacing: 2) {
                        Text(loc.country)
                            .font(.title2.weight(.semibold))
                        Text(tagline(loc.code))
                            .font(.caption).foregroundStyle(.secondary)
                        if !loc.available {
                            Label("Gateway offline", systemImage: "exclamationmark.triangle.fill")
                                .font(.caption2).foregroundStyle(.orange)
                        }
                    }
                    Spacer()
                    // Latency badge when connected
                    if model.phase == .connected && model.latencyText != "—" {
                        VStack(spacing: 1) {
                            Text(model.latencyText)
                                .font(.system(.body, design: .monospaced).weight(.bold))
                                .foregroundStyle(Theme.accent)
                            Text("latency").font(.caption2).foregroundStyle(.secondary)
                        }
                    }
                }
            } else {
                HStack(spacing: 10) {
                    Text("🌍").font(.system(size: 36))
                    Text("Pick a country below to connect")
                        .font(.subheadline).foregroundStyle(.secondary)
                    Spacer()
                }
            }

            // Big connect / disconnect button
            Button(action: {
                switch model.phase {
                case .connected, .connecting: model.disconnect()
                default: model.connectSelected()
                }
            }) {
                HStack(spacing: 8) {
                    if model.phase == .connecting {
                        ProgressView().controlSize(.small).tint(.white)
                    } else {
                        Image(systemName: model.phase == .connected ? "power" : "network.badge.shield.half.filled")
                    }
                    Text(buttonLabel)
                        .font(.headline)
                }
                .frame(maxWidth: .infinity).frame(height: 50)
            }
            .buttonStyle(.borderedProminent)
            .tint(model.phase == .connected ? Color.red : Theme.accent)
            .disabled(model.phase != .connected && model.phase != .connecting && model.selectedCountry == nil)
            .animation(.easeInOut(duration: 0.2), value: model.phase)

            // Stats row — only when connected
            if model.phase == .connected {
                StatsBar()
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .padding(18)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 18))
        .shadow(color: .black.opacity(0.07), radius: 12, y: 4)
        .animation(.spring(duration: 0.35), value: model.phase == .connected)
    }

    private var buttonLabel: String {
        switch model.phase {
        case .connected: return "Disconnect"
        case .connecting: return "Connecting…"
        default: return "Connect"
        }
    }
}

// ─────────────────────────────────────────────
// MARK: - Stats bar (connected)
// ─────────────────────────────────────────────
private struct StatsBar: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        HStack(spacing: 0) {
            stat(icon: "clock", label: "Duration",  value: model.durationText)
            Divider().frame(height: 32)
            stat(icon: "arrow.down", label: "Down",     value: model.downloadText)
            Divider().frame(height: 32)
            stat(icon: "arrow.up",   label: "Up",       value: model.uploadText)
            Divider().frame(height: 32)
            stat(icon: "waveform.path.ecg", label: "Ping", value: model.latencyText)
        }
        .padding(10)
        .background(Theme.accent.opacity(0.06), in: RoundedRectangle(cornerRadius: 12))
    }

    private func stat(icon: String, label: String, value: String) -> some View {
        VStack(spacing: 3) {
            Image(systemName: icon)
                .font(.caption2).foregroundStyle(Theme.accent)
            Text(value)
                .font(.system(.caption, design: .monospaced).weight(.semibold))
                .monospacedDigit()
            Text(label)
                .font(.system(size: 9)).foregroundStyle(.tertiary)
        }
        .frame(maxWidth: .infinity)
    }
}

// ─────────────────────────────────────────────
// MARK: - Country grid
// ─────────────────────────────────────────────
private struct CountryGrid: View {
    @EnvironmentObject var model: AppModel
    @State private var isRefreshing = false

    private let columns = [
        GridItem(.adaptive(minimum: 140, maximum: 180), spacing: 10)
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Server Locations")
                    .font(.subheadline.weight(.semibold))
                Spacer()
                Button {
                    isRefreshing = true
                    model.loadLocations()
                    DispatchQueue.main.asyncAfter(deadline: .now() + 1) { isRefreshing = false }
                } label: {
                    Image(systemName: "arrow.clockwise")
                        .rotationEffect(.degrees(isRefreshing ? 360 : 0))
                        .animation(isRefreshing ? .linear(duration: 0.7).repeatForever(autoreverses: false) : .default,
                                   value: isRefreshing)
                }
                .buttonStyle(.borderless)
                .help("Refresh locations")
            }

            if model.locations.isEmpty {
                HStack {
                    Spacer()
                    VStack(spacing: 8) {
                        ProgressView()
                        Text("Loading locations…").font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                }
                .padding(.vertical, 30)
            } else {
                LazyVGrid(columns: columns, spacing: 10) {
                    ForEach(model.locations, id: \.code) { loc in
                        CountryCard(loc: loc)
                    }
                }
            }
        }
    }
}

// ─────────────────────────────────────────────
// MARK: - Country card
// ─────────────────────────────────────────────
private struct CountryCard: View {
    @EnvironmentObject var model: AppModel
    let loc: FfiLocation

    private var isSelected: Bool { model.selectedCountry == loc.code }
    private var isConnected: Bool { isSelected && model.phase == .connected }

    var body: some View {
        Button {
            guard model.phase != .connected && model.phase != .connecting else { return }
            model.selectedCountry = loc.code
        } label: {
            VStack(alignment: .leading, spacing: 6) {
                HStack {
                    Text(flag(loc.code)).font(.system(size: 28))
                    Spacer()
                    Circle()
                        .fill(loc.available ? Color.green : Color.orange)
                        .frame(width: 8, height: 8)
                }
                Text(loc.country)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                Text(tagline(loc.code))
                    .font(.system(size: 10))
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: 4) {
                    Image(systemName: "server.rack")
                        .font(.system(size: 9))
                    Text("\(loc.servers) server\(loc.servers == 1 ? "" : "s")")
                        .font(.system(size: 9))
                }
                .foregroundStyle(.tertiary)
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(cardBackground, in: RoundedRectangle(cornerRadius: 14))
            .overlay(
                RoundedRectangle(cornerRadius: 14)
                    .stroke(isSelected ? Theme.accent : Color.clear, lineWidth: 2)
            )
            .shadow(color: isSelected ? Theme.accent.opacity(0.2) : .black.opacity(0.04),
                    radius: isSelected ? 8 : 3, y: 2)
            .scaleEffect(isSelected ? 1.02 : 1.0)
            .animation(.spring(duration: 0.25), value: isSelected)
        }
        .buttonStyle(.plain)
        .disabled(!loc.available && model.phase != .connected)
        .opacity(loc.available ? 1.0 : 0.55)
    }

    private var cardBackground: some ShapeStyle {
        if isConnected {
            return AnyShapeStyle(LinearGradient(
                colors: [Theme.accent.opacity(0.18), Theme.accent.opacity(0.08)],
                startPoint: .topLeading, endPoint: .bottomTrailing
            ))
        } else if isSelected {
            return AnyShapeStyle(Theme.accent.opacity(0.10))
        } else {
            return AnyShapeStyle(Color.secondary.opacity(0.07))
        }
    }
}

// ─────────────────────────────────────────────
// MARK: - Preview
// ─────────────────────────────────────────────
#if DEBUG
#Preview {
    ContentView()
        .environmentObject(AppModel())
}
#endif
