import NetworkExtension
import WireGuardKit
import os

/// The packet-tunnel extension. It reads the WireGuard configuration the app
/// placed in the keychain (referenced from the provider configuration) and
/// establishes a real WireGuard tunnel via WireGuardKit (wireguard-apple). This
/// is the actual OS-level VPN — no custom protocol, no mock.
class PacketTunnelProvider: NEPacketTunnelProvider {
    private lazy var adapter: WireGuardAdapter = {
        WireGuardAdapter(with: self) { _, message in
            os_log("wg: %{public}s", message)
        }
    }()

    override func startTunnel(options _: [String: NSObject]?,
                              completionHandler: @escaping (Error?) -> Void) {
        guard let proto = protocolConfiguration as? NETunnelProviderProtocol,
              let ref = proto.providerConfiguration?["wgQuickRef"] as? Data,
              let wgQuick = Keychain.resolve(ref: ref) else {
            completionHandler(PacketTunnelError.missingConfiguration)
            return
        }

        let config: TunnelConfiguration
        do {
            config = try TunnelConfiguration(fromWgQuickConfig: wgQuick, called: "Aevora")
        } catch {
            completionHandler(PacketTunnelError.invalidConfiguration(error))
            return
        }

        adapter.start(tunnelConfiguration: config) { adapterError in
            if let adapterError {
                os_log("wg adapter start failed: %{public}s", "\(adapterError)")
                completionHandler(adapterError)
            } else {
                completionHandler(nil)
            }
        }
    }

    override func stopTunnel(with _: NEProviderStopReason,
                             completionHandler: @escaping () -> Void) {
        adapter.stop { _ in completionHandler() }
    }

    /// The app can request runtime stats (bytes transferred) via a provider
    /// message; returns the WireGuard runtime configuration string.
    override func handleAppMessage(_ messageData: Data,
                                   completionHandler: ((Data?) -> Void)?) {
        guard String(data: messageData, encoding: .utf8) == "stats" else {
            completionHandler?(nil); return
        }
        adapter.getRuntimeConfiguration { config in
            completionHandler?(config?.data(using: .utf8))
        }
    }
}

enum PacketTunnelError: Error {
    case missingConfiguration
    case invalidConfiguration(Error)
}
