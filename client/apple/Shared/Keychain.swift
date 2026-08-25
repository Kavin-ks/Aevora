import Foundation

/// Keychain helper shared by the app and the packet-tunnel extension. The
/// WireGuard config (which contains the device private key) is stored here and
/// the extension receives only a persistent reference, so the secret is not kept
/// in the NetworkExtension preferences. Cross-process resolution requires the app
/// and extension to share a keychain access group (see the entitlements).
enum Keychain {
    private static let service = "com.aevora.wg"

    /// Stores `value` for `account` and returns a persistent reference.
    static func store(_ value: String, account: String) -> Data? {
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(base as CFDictionary)

        var add = base
        add[kSecValueData as String] = value.data(using: .utf8)!
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        add[kSecReturnPersistentRef as String] = true
        var result: CFTypeRef?
        guard SecItemAdd(add as CFDictionary, &result) == errSecSuccess else { return nil }
        return result as? Data
    }

    /// Resolves a persistent reference back to its stored string value.
    static func resolve(ref: Data) -> String? {
        let query: [String: Any] = [
            kSecValuePersistentRef as String: ref,
            kSecReturnData as String: true,
        ]
        var out: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    /// Reads the raw string stored for an account (no persistent ref).
    static func read(account: String) -> String? {
        let q: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
        ]
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func delete(account: String) {
        let q: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(q as CFDictionary)
    }
}
