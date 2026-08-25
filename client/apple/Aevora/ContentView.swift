import SwiftUI
import aevora_core

// Deliberately minimal per Phase 2b ("first make the tunnel work reliably").
// The world map and polished consumer UI come later; this exercises the full
// enroll -> locations -> connect -> status -> disconnect path.

struct ContentView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 16) {
            Text("Aevora").font(.largeTitle.bold())

            if !model.isEnrolled {
                EnrollView()
            } else {
                StatusView()
                LocationsView()
            }

            if let err = model.lastError {
                Text(err).font(.footnote).foregroundStyle(.red).lineLimit(3)
            }
        }
        .padding(24)
        .frame(minWidth: 380, minHeight: 460)
        .onAppear { if model.isEnrolled { model.loadLocations() } }
    }
}

private struct EnrollView: View {
    @EnvironmentObject var model: AppModel
    @State private var invite = ""
    @State private var email = ""

    var body: some View {
        VStack(spacing: 10) {
            Text("Enroll this device").font(.headline)
            TextField("Invite code", text: $invite).textFieldStyle(.roundedBorder)
            TextField("Email", text: $email).textFieldStyle(.roundedBorder)
            Button("Enroll") {
                model.enroll(invite: invite, email: email, deviceName: Host.current().localizedName ?? "Mac")
            }
            .disabled(invite.isEmpty || email.isEmpty)
        }
    }
}

private struct StatusView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(spacing: 6) {
            Text(stateText).font(.title2.bold()).foregroundStyle(stateColor)
            if model.phase == .connected {
                Text(model.serverName).font(.subheadline)
                Grid(horizontalSpacing: 24, verticalSpacing: 4) {
                    GridRow { Text("Duration"); Text(model.durationText).monospacedDigit() }
                    GridRow { Text("Latency"); Text(model.latencyText) }
                    GridRow { Text("Download"); Text(model.downloadText) }
                    GridRow { Text("Upload"); Text(model.uploadText) }
                }
                .font(.footnote).foregroundStyle(.secondary)
                Button("Disconnect") { model.disconnect() }.tint(.red)
            }
        }
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

private struct LocationsView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Locations").font(.headline)
            List(model.locations, id: \.code) { loc in
                HStack {
                    Text(loc.country)
                    Spacer()
                    if loc.available {
                        Button("Connect") { model.connect(country: loc.code) }
                            .disabled(model.phase == .connecting || model.phase == .connected)
                    } else {
                        Text("unavailable").foregroundStyle(.secondary).font(.caption)
                    }
                }
            }
            .frame(minHeight: 160)
            Button("Refresh") { model.loadLocations() }.font(.caption)
        }
    }
}
