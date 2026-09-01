import SwiftUI

// Consumer layout: Aevora wordmark, world map, a selected-country panel with a
// large Connect/Disconnect button, connection state, and live stats. Original
// Aevora identity — no third-party branding.

struct ContentView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 18) {
            HStack(spacing: 4) {
                Text("Aevora").font(.system(size: 28, weight: .bold))
                Text("•").font(.system(size: 28, weight: .bold)).foregroundStyle(Theme.accent)
            }

            if !model.isEnrolled {
                EnrollView()
            } else {
                WorldMapView()
                ControlPanel()
                LocationPicker()
            }

            if let err = model.lastError {
                Text(err).font(.footnote).foregroundStyle(.red).lineLimit(3)
            }
            Spacer(minLength: 0)
        }
        .padding(22)
        .frame(minWidth: 440, minHeight: 620)
        .onAppear { if model.isEnrolled { model.loadLocations() } }
    }
}

private struct EnrollView: View {
    @EnvironmentObject var model: AppModel
    @State private var invite = ""
    @State private var email = ""

    var body: some View {
        VStack(spacing: 12) {
            Text("Enroll this device").font(.headline)
            TextField("Invite code", text: $invite).textFieldStyle(.roundedBorder)
            TextField("Email", text: $email).textFieldStyle(.roundedBorder)
            Button("Enroll") {
                model.enroll(invite: invite, email: email, deviceName: Platform.deviceName)
            }
            .buttonStyle(.borderedProminent).tint(Theme.accent)
            .disabled(invite.isEmpty || email.isEmpty)
        }
        .frame(maxWidth: 320)
        .padding(.top, 40)
    }
}

private struct ControlPanel: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 10) {
            Text(stateText)
                .font(.title3.weight(.semibold))
                .foregroundStyle(stateColor)

            Text(model.phase == .connected ? model.serverName
                 : (model.selectedCountryName ?? "Select a location"))
                .font(.subheadline).foregroundStyle(.secondary)

            Button(action: primaryAction) {
                Text(primaryLabel)
                    .font(.headline)
                    .frame(maxWidth: .infinity).frame(height: 46)
            }
            .buttonStyle(.borderedProminent)
            .tint(model.phase == .connected ? .red : Theme.accent)
            .disabled(primaryDisabled)

            if model.phase == .connected {
                StatsGrid()
            }
        }
        .padding(16)
        .background(RoundedRectangle(cornerRadius: 14).fill(Color.secondary.opacity(0.08)))
    }

    private func primaryAction() {
        switch model.phase {
        case .connected, .connecting: model.disconnect()
        default: model.connectSelected()
        }
    }
    private var primaryLabel: String {
        switch model.phase {
        case .connected: return "Disconnect"
        case .connecting: return "Cancel"
        default: return "Connect"
        }
    }
    private var primaryDisabled: Bool {
        model.phase != .connected && model.phase != .connecting && model.selectedCountry == nil
    }
    private var stateText: String {
        switch model.phase {
        case .connected: return "CONNECTED"
        case .connecting: return "CONNECTING…"
        case .failed: return "FAILED"
        default: return "DISCONNECTED"
        }
    }
    private var stateColor: Color {
        switch model.phase {
        case .connected: return .green
        case .failed: return .red
        default: return .secondary
        }
    }
}

private struct StatsGrid: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        Grid(horizontalSpacing: 28, verticalSpacing: 6) {
            GridRow { label("Duration"); value(model.durationText).monospacedDigit() }
            GridRow { label("Latency"); value(model.latencyText) }
            GridRow { label("Download"); value(model.downloadText) }
            GridRow { label("Upload"); value(model.uploadText) }
        }
        .padding(.top, 6)
    }
    private func label(_ s: String) -> some View { Text(s).font(.caption).foregroundStyle(.secondary) }
    private func value(_ s: String) -> Text { Text(s).font(.caption.weight(.medium)) }
}

private struct LocationPicker: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        HStack {
            Picker("Location", selection: Binding(
                get: { model.selectedCountry ?? "" },
                set: { model.selectedCountry = $0.isEmpty ? nil : $0 }
            )) {
                Text("Select…").tag("")
                ForEach(model.locations, id: \.code) { loc in
                    Text(loc.available ? loc.country : "\(loc.country) (unavailable)")
                        .tag(loc.code)
                }
            }
            .disabled(model.phase == .connected || model.phase == .connecting)
            Button {
                model.loadLocations()
            } label: { Image(systemName: "arrow.clockwise") }
            .help("Refresh locations")
        }
    }
}
