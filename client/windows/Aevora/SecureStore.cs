using System;
using System.IO;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace Aevora;

// Persists the session (device id, refresh token, WireGuard private key) encrypted
// with DPAPI (CurrentUser). The device private key never leaves the machine; only
// the public key is sent to the control plane.
internal static class SecureStore
{
    private static readonly string Path = System.IO.Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Aevora", "session.dat");

    public static void Save(Session s)
    {
        Directory.CreateDirectory(System.IO.Path.GetDirectoryName(Path)!);
        byte[] plain = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(s));
        byte[] enc = ProtectedData.Protect(plain, null, DataProtectionScope.CurrentUser);
        File.WriteAllBytes(Path, enc);
    }

    public static Session? Load()
    {
        if (!File.Exists(Path)) return null;
        try
        {
            byte[] enc = File.ReadAllBytes(Path);
            byte[] plain = ProtectedData.Unprotect(enc, null, DataProtectionScope.CurrentUser);
            return JsonSerializer.Deserialize<Session>(Encoding.UTF8.GetString(plain));
        }
        catch { return null; }
    }

    public static void Clear()
    {
        try { File.Delete(Path); } catch { }
    }
}
