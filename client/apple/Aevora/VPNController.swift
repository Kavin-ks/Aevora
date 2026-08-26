import Foundation
import NetworkExtension
import aevora_core

/// Drives the system VPN via NetworkExtension. It installs an on-demand tunnel
/// configuration pointing at our packet-tunnel extension, passes the WireGuard
/// config securely (keychain persistent reference), starts/stops the tunnel, and
/// reports OS-level status changes back to the caller.
final class VPNController {
    /// Called on the main queue whenever the OS VPN status changes.
    var onStatusChange: ((NEVPNStatus) -> Void)?

    private var manager: NETunnelProviderManager?
    private let tunnelAccount = "aevora.tunnel"

    init() {
        NotificationCenter.default.addObserver(
            self, selector: #selector(statusChanged(_:)),
            name: .NEVPNStatusDidChange, object: nil)
    }

    /// Loads (or creates) the single Aevora tunnel manager.
    func loadManager() async throws -> NETunnelProviderManager {
        let all = try await NETunnelProviderManager.loadAllFromPreferences()
        let mgr = all.first ?? NETunnelProviderManager()
        self.manager = mgr
        return mgr
    }

    /// Installs the config from the core and starts the tunnel. The private key
    /// (inside the wg-quick config) is stored in the keychain; only a persistent
    /// reference is placed in the NE provider configuration.
    func start(config: FfiTunnelConfig, description: String) async throws {
        let mgr = try await loadManager()

        let wg = Self.wgQuick(config)
        guard let ref = Keychain.store(wg, account: tunnelAccount) else {
            throw VPNError.keychain
        }

        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = AppConfig.tunnelBundleId
        proto.serverAddress = config.endpoint
        proto.providerConfiguration = ["wgQuickRef": ref]

        mgr.protocolConfiguration = proto
        mgr.localizedDescription = description
        mgr.isEnabled = true

        try await mgr.saveToPreferences()
        // Reload so the connection object reflects the saved configuration.
        try await mgr.loadFromPreferences()

        try mgr.connection.startVPNTunnel()
    }

    /// Stops the tunnel.
    func stop() {
        manager?.connection.stopVPNTunnel()
        Keychain.delete(account: tunnelAccount)
    }

    var status: NEVPNStatus { manager?.connection.status ?? .invalid }

    /// Asks the extension for the tunnel's cumulative byte counters (real
    /// WireGuard rx/tx from `getRuntimeConfiguration`, in UAPI format). Returns
    /// nil if unavailable.
    func fetchTunnelStats() async -> (rx: UInt64, tx: UInt64)? {
        guard let session = manager?.connection as? NETunnelProviderSession,
              let request = "stats".data(using: .utf8) else { return nil }
        return await withCheckedContinuation { cont in
            do {
                try session.sendProviderMessage(request) { data in
                    guard let data, let text = String(data: data, encoding: .utf8) else {
                        cont.resume(returning: nil); return
                    }
                    cont.resume(returning: Self.parseWgStats(text))
                }
            } catch {
                cont.resume(returning: nil)
            }
        }
    }

    /// Sums rx_bytes/tx_bytes across peers from WireGuard's UAPI runtime config.
    static func parseWgStats(_ uapi: String) -> (rx: UInt64, tx: UInt64) {
        var rx: UInt64 = 0, tx: UInt64 = 0
        for line in uapi.split(separator: "\n") {
            let kv = line.split(separator: "=", maxSplits: 1)
            guard kv.count == 2 else { continue }
            if kv[0] == "rx_bytes", let v = UInt64(kv[1]) { rx += v }
            if kv[0] == "tx_bytes", let v = UInt64(kv[1]) { tx += v }
        }
        return (rx, tx)
    }

    @objc private func statusChanged(_ note: Notification) {
        let status = self.manager?.connection.status ?? .invalid
        DispatchQueue.main.async { self.onStatusChange?(status) }
    }

    /// Renders a wg-quick config string the extension can parse with WireGuardKit.
    static func wgQuick(_ c: FfiTunnelConfig) -> String {
        var lines = [
            "[Interface]",
            "PrivateKey = \(c.privateKey)",
            "Address = \(c.addresses.joined(separator: ", "))",
        ]
        if !c.dns.isEmpty {
            lines.append("DNS = \(c.dns.joined(separator: ", "))")
        }
        lines.append("")
        lines.append("[Peer]")
        lines.append("PublicKey = \(c.peerPublicKey)")
        lines.append("Endpoint = \(c.endpoint)")
        lines.append("AllowedIPs = \(c.allowedIps.joined(separator: ", "))")
        lines.append("PersistentKeepalive = \(c.persistentKeepalive)")
        return lines.joined(separator: "\n")
    }
}

enum VPNError: Error {
    case keychain
}
