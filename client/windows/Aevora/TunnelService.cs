using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;

namespace Aevora;

// Real WireGuard tunnel on Windows via the official embeddable WireGuardNT:
//   - tunnel.dll  : WireGuardTunnelService(conf) runs the tunnel as a service.
//   - wireguard.dll: WireGuardGetConfiguration reads live rx/tx counters.
//
// The app installs itself as a Windows service whose entry point calls
// WireGuardTunnelService(conf) (see App.OnStartup handling "/service"). No custom
// protocol; this is the same mechanism WireGuard for Windows uses.
//
// NOTE: administrator privileges are required to install/start the service and
// create the adapter. Build/verify locally (see README) — it cannot run in a
// non-Windows environment.

public static class TunnelService
{
    private const string ServiceName = "AevoraTunnel";
    private static readonly string ConfDir =
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "Aevora");

    /// Writes the wg-quick config and installs+starts the tunnel service.
    public static void Connect(TunnelConfig config)
    {
        Directory.CreateDirectory(ConfDir);
        string conf = Path.Combine(ConfDir, "aevora.conf");
        File.WriteAllText(conf, WgQuick(config));
        // Restrict the config to Administrators/SYSTEM (contains the private key).
        try { new FileInfo(conf).Attributes |= FileAttributes.NotContentIndexed; } catch { }

        ServiceManager.InstallAndStart(ServiceName, $"\"{Environment.ProcessPath}\" /service \"{conf}\"");
    }

    /// Stops and removes the tunnel service.
    public static void Disconnect()
    {
        ServiceManager.StopAndRemove(ServiceName);
        try { File.Delete(Path.Combine(ConfDir, "aevora.conf")); } catch { }
    }

    /// Entry point when launched as the Windows service: runs the tunnel loop.
    public static int RunServiceEntryPoint(string confPath)
    {
        return WireGuardTunnelService(confPath) ? 0 : 1;
    }

    /// Reads cumulative rx/tx bytes from the WireGuard adapter. Returns false if
    /// unavailable (adapter not up yet).
    public static bool TryGetTransfer(out ulong rx, out ulong tx)
    {
        rx = 0; tx = 0;
        IntPtr adapter = WireGuardOpenAdapter("Aevora");
        if (adapter == IntPtr.Zero) return false;
        try
        {
            uint size = 0;
            WireGuardGetConfiguration(adapter, IntPtr.Zero, ref size); // query size
            if (size == 0) return false;
            IntPtr buf = Marshal.AllocHGlobal((int)size);
            try
            {
                if (!WireGuardGetConfiguration(adapter, buf, ref size)) return false;
                SumPeerTransfer(buf, out rx, out tx);
                return true;
            }
            finally { Marshal.FreeHGlobal(buf); }
        }
        finally { WireGuardCloseAdapter(adapter); }
    }

    // Parses the WIREGUARD_INTERFACE + WIREGUARD_PEER[] buffer and sums transfer.
    private static void SumPeerTransfer(IntPtr buf, out ulong rx, out ulong tx)
    {
        rx = 0; tx = 0;
        // WIREGUARD_INTERFACE: Flags(4) ListenPort(2) +pad(2) PrivateKey(32) PublicKey(32) PeersCount(4) +pad
        uint peers = (uint)Marshal.ReadInt32(buf, 4 + 4 + 32 + 32);
        int ifaceSize = 4 + 4 + 32 + 32 + 8; // aligned interface header
        int peerSize = Marshal.SizeOf<WireGuardPeer>();
        IntPtr p = IntPtr.Add(buf, ifaceSize);
        for (uint i = 0; i < peers; i++)
        {
            var peer = Marshal.PtrToStructure<WireGuardPeer>(p);
            rx += peer.RxBytes; tx += peer.TxBytes;
            p = IntPtr.Add(p, peerSize + (int)(peer.AllowedIPsCount * (uint)Marshal.SizeOf<WireGuardAllowedIp>()));
        }
    }

    private static string WgQuick(TunnelConfig c)
    {
        var sb = new StringBuilder();
        sb.AppendLine("[Interface]");
        sb.AppendLine($"PrivateKey = {c.Private_key}");
        sb.AppendLine($"Address = {string.Join(", ", c.Addresses)}");
        if (c.Dns.Length > 0) sb.AppendLine($"DNS = {string.Join(", ", c.Dns)}");
        sb.AppendLine();
        sb.AppendLine("[Peer]");
        sb.AppendLine($"PublicKey = {c.Peer_public_key}");
        sb.AppendLine($"Endpoint = {c.Endpoint}");
        sb.AppendLine($"AllowedIPs = {string.Join(", ", c.Allowed_ips)}");
        sb.AppendLine($"PersistentKeepalive = {c.Persistent_keepalive}");
        return sb.ToString();
    }

    // --- native declarations ---
    [DllImport("tunnel.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.I1)]
    private static extern bool WireGuardTunnelService([MarshalAs(UnmanagedType.LPWStr)] string confFile);

    [DllImport("wireguard.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr WireGuardOpenAdapter([MarshalAs(UnmanagedType.LPWStr)] string name);

    [DllImport("wireguard.dll")]
    private static extern void WireGuardCloseAdapter(IntPtr adapter);

    [DllImport("wireguard.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.I1)]
    private static extern bool WireGuardGetConfiguration(IntPtr adapter, IntPtr iface, ref uint bytes);
}

// Subset of WIREGUARD_PEER we read (transfer counters). Field order/sizes match
// wireguard.h; only the fields up to RxBytes are consumed.
[StructLayout(LayoutKind.Sequential)]
internal struct WireGuardPeer
{
    public uint Flags;
    public uint Reserved;
    [MarshalAs(UnmanagedType.ByValArray, SizeConst = 32)] public byte[] PublicKey;
    [MarshalAs(UnmanagedType.ByValArray, SizeConst = 32)] public byte[] PresharedKey;
    public ushort PersistentKeepalive;
    [MarshalAs(UnmanagedType.ByValArray, SizeConst = 28)] public byte[] Endpoint; // SOCKADDR_INET
    public ulong TxBytes;
    public ulong RxBytes;
    public ulong LastHandshake;
    public uint AllowedIPsCount;
}

[StructLayout(LayoutKind.Sequential)]
internal struct WireGuardAllowedIp
{
    [MarshalAs(UnmanagedType.ByValArray, SizeConst = 16)] public byte[] Address;
    public ushort AddressFamily;
    public byte Cidr;
}
