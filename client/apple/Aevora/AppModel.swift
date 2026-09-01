import Foundation
import NetworkExtension
// aevora_core types are compiled as part of this module (build/Generated/aevora_core.swift)
// — no separate import needed.

/// The app's view model. It owns the shared `AevoraClient` (all API/auth/session/
/// key/selection logic) and the `VPNController` (native tunnel), and exposes a
/// small amount of observable state for the UI. No VPN/business logic is
/// duplicated here — this class only orchestrates and formats for display.
@MainActor
final class AppModel: ObservableObject {
    enum Phase: Equatable {
        case needsEnrollment
        case disconnected
        case connecting
        case connected
        case failed(String)
    }

    @Published var phase: Phase = .needsEnrollment
    @Published var locations: [FfiLocation] = []
    @Published var selectedCountry: String?
    @Published var serverName: String = ""
    @Published var durationSeconds: Int = 0
    @Published var lastError: String?

    // Real stats from the core: download/upload/duration from the OS tunnel's
    // byte counters, and latency from an in-tunnel TCP probe to the gateway
    // (measured by the core). Shown as "—" only before the first sample; never faked.
    @Published var latencyText: String = "—"
    @Published var downloadText: String = "—"
    @Published var uploadText: String = "—"

    private let client: AevoraClient
    private let vpn = VPNController()
    private var connectedAt: Date?
    private var timer: Timer?
    private var tickCount = 0

    init() {
        client = AevoraClient(baseUrl: AppConfig.controlURL)
        if let session = SessionStore.load() {
            client.restore(session: session)
            phase = .disconnected
        }
        vpn.onStatusChange = { [weak self] status in
            self?.handleStatus(status)
        }
    }

    var isEnrolled: Bool { phase != .needsEnrollment }

    // MARK: Actions

    func enroll(invite: String, email: String, deviceName: String) {
        run {
            let session = try self.client.enroll(
                inviteCode: invite, email: email, displayName: nil,
                deviceName: deviceName, platform: Platform.tag)
            SessionStore.save(session)
        } then: {
            self.phase = .disconnected
            self.loadLocations()
        }
    }

    func loadLocations() {
        run {
            try self.client.locations()
        } thenValue: { locs in
            self.locations = locs
        }
    }

    func connect(country: String) {
        selectedCountry = country
        phase = .connecting
        run {
            // The core selects a gateway and leases an address; it returns the
            // WireGuard config the native tunnel establishes.
            let conn = try self.client.prepareConnection(countryCode: country)
            return conn
        } thenValue: { conn in
            self.serverName = conn.serverName
            Task {
                do {
                    try await self.vpn.start(config: conn.config,
                                             description: "Aevora — \(conn.serverName)")
                } catch {
                    self.client.markFailed(reason: "tunnel start failed")
                    self.phase = .failed(error.localizedDescription)
                }
            }
        }
    }

    /// Connects to the country currently selected on the map/list.
    func connectSelected() {
        if let c = selectedCountry { connect(country: c) }
    }

    /// The display name of the selected country, if any.
    var selectedCountryName: String? {
        guard let code = selectedCountry else { return nil }
        return locations.first(where: { $0.code == code })?.country
    }

    func disconnect() {
        vpn.stop()
        run {
            try self.client.disconnect()
        } then: {
            self.stopTimer()
            self.phase = .disconnected
            self.serverName = ""
        }
    }

    // MARK: OS status -> core + UI

    private func handleStatus(_ status: NEVPNStatus) {
        switch status {
        case .connected:
            client.markConnected()
            connectedAt = Date()
            startTimer()
            phase = .connected
        case .disconnected:
            if phase == .connecting {
                // The tunnel dropped before it came up.
                client.markFailed(reason: "tunnel disconnected")
                phase = .failed("Tunnel disconnected")
            } else {
                phase = .disconnected
            }
            stopTimer()
        case .reasserting, .connecting:
            phase = .connecting
        default:
            break
        }
    }

    private func startTimer() {
        stopTimer()
        timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.tick() }
        }
    }

    private func tick() {
        if let start = connectedAt {
            durationSeconds = Int(Date().timeIntervalSince(start))
        }
        // Every 3s, fetch the real tunnel counters and feed them to the core,
        // which computes download/upload rates and renews the lease.
        tickCount += 1
        if tickCount % 3 == 0 {
            Task.detached(priority: .utility) { [weak self] in
                guard let self, let (rx, tx) = await self.vpn.fetchTunnelStats() else { return }
                if let stats = try? self.client.reportTunnelStats(rxBytes: rx, txBytes: tx, latencyMs: nil) {
                    await MainActor.run { self.applyStats(stats) }
                }
            }
        }
    }

    private func applyStats(_ s: FfiStats) {
        downloadText = Self.formatRate(s.downloadBps)
        uploadText = Self.formatRate(s.uploadBps)
        latencyText = s.latencyMs > 0 ? "\(s.latencyMs) ms" : "—"
    }

    /// Formats a byte/s rate for display (Mbps for consumer feel, KB/s when small).
    static func formatRate(_ bytesPerSec: UInt64) -> String {
        let mbps = Double(bytesPerSec) * 8 / 1_000_000
        if mbps >= 1 { return String(format: "%.1f Mbps", mbps) }
        let kbps = Double(bytesPerSec) / 1024
        return String(format: "%.0f KB/s", kbps)
    }

    private func stopTimer() {
        timer?.invalidate(); timer = nil; connectedAt = nil; durationSeconds = 0
        tickCount = 0
        downloadText = "—"; uploadText = "—"; latencyText = "—"
    }

    // MARK: Core call helpers (the core is blocking; run off the main thread)

    private func run(_ work: @escaping () throws -> Void, then done: @escaping () -> Void) {
        Task.detached(priority: .userInitiated) {
            do { try work(); await MainActor.run { self.lastError = nil; done() } }
            catch { await MainActor.run { self.fail(error) } }
        }
    }

    private func run<T>(_ work: @escaping () throws -> T, thenValue done: @escaping (T) -> Void) {
        Task.detached(priority: .userInitiated) {
            do { let v = try work(); await MainActor.run { self.lastError = nil; done(v) } }
            catch { await MainActor.run { self.fail(error) } }
        }
    }

    private func fail(_ error: Error) {
        lastError = "\(error)"
        if phase == .connecting { phase = .failed("\(error)") }
    }

    var durationText: String {
        let s = durationSeconds
        return String(format: "%02d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
    }
}
