import Foundation
#if os(iOS)
import UIKit
#endif

// MARK: - Platform
//
// The one place OS differences for identity are expressed; the rest of the app
// is shared between macOS and iOS.

enum Platform {
    /// Platform tag sent at enrollment (matches the control plane's allowed set).
    static var tag: String {
        #if os(iOS)
        return "ios"
        #else
        return "macos"
        #endif
    }

    /// A human-readable device name for enrollment.
    static var deviceName: String {
        #if os(iOS)
        return UIDevice.current.name
        #else
        return Host.current().localizedName ?? "Mac"
        #endif
    }
}

// MARK: - Configuration
//
// The control-plane URL is injected via a build setting (AEVORA_CONTROL_URL ->
// Info.plist), never hardcoded, so dev/staging/prod endpoints are not baked into
// the binary. Set it in the xcconfig or the scheme environment.

enum AppConfig {
    static var controlURL: String {
        (Bundle.main.object(forInfoDictionaryKey: "AEVORA_CONTROL_URL") as? String)?
            .trimmingCharacters(in: .whitespaces) ?? ""
    }

    /// Bundle id of the packet-tunnel extension.
    static var tunnelBundleId: String {
        (Bundle.main.bundleIdentifier ?? "com.aevora.Aevora") + ".PacketTunnel"
    }
}

// MARK: - Session persistence
//
// The FfiSession (device id, refresh token, and the WireGuard private key) is
// persisted in the keychain so the app can `restore` it on next launch. Only the
// public key ever leaves the device.

enum SessionStore {
    private static let account = "aevora.session"

    static func save(_ s: FfiSession) {
        let blob = [s.deviceId, s.userId, s.refreshToken, s.privateKey, s.publicKey]
            .joined(separator: "\n")
        _ = Keychain.store(blob, account: account)
    }

    static func load() -> FfiSession? {
        guard let blob = Keychain.read(account: account) else { return nil }
        let f = blob.components(separatedBy: "\n")
        guard f.count == 5 else { return nil }
        return FfiSession(deviceId: f[0], userId: f[1], refreshToken: f[2], privateKey: f[3], publicKey: f[4])
    }

    static func clear() { Keychain.delete(account: account) }
}
