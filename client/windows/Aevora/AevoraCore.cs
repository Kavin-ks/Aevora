using System;
using System.Runtime.InteropServices;
using System.Text.Json;

namespace Aevora;

// P/Invoke wrapper around the shared Rust core's C ABI (aevora_core.dll, built
// with the `capi` feature). ALL auth/API/selection/lease/key/stats logic lives
// in Rust; this class only marshals. Complex results come back as JSON strings.

public sealed class AevoraCore : IDisposable
{
    private const string Dll = "aevora_core";
    private IntPtr _handle;

    public AevoraCore(string baseUrl)
    {
        _handle = aevora_client_new(baseUrl);
        if (_handle == IntPtr.Zero) throw new InvalidOperationException("failed to create core client");
    }

    public Session Enroll(string invite, string email, string deviceName)
        => Parse<Session>(Call(h => aevora_enroll(h, invite, email, deviceName)));

    public void Restore(Session s)
        => ThrowIfError(Call(h => aevora_restore(h, JsonSerializer.Serialize(s))));

    public Location[] Locations()
        => Parse<LocationsResponse>(Call(h => aevora_locations(h))).Locations;

    public Connection PrepareConnection(string countryCode)
        => Parse<Connection>(Call(h => aevora_prepare_connection(h, countryCode)));

    public void MarkConnected() => aevora_mark_connected(_handle);

    public void Disconnect() => ThrowIfError(Call(h => aevora_disconnect(h)));

    public Stats ReportStats(ulong rxBytes, ulong txBytes)
        => Parse<Stats>(Call(h => aevora_report_stats(h, rxBytes, txBytes)));

    public Stats CurrentStats() => Parse<Stats>(Call(h => aevora_current_stats(h)));

    public string State() => Parse<StateResponse>(Call(h => aevora_state(h))).State;

    // --- marshaling helpers ---

    private string Call(Func<IntPtr, IntPtr> f)
    {
        IntPtr ptr = f(_handle);
        try { return Marshal.PtrToStringUTF8(ptr) ?? "{}"; }
        finally { if (ptr != IntPtr.Zero) aevora_string_free(ptr); }
    }

    private static T Parse<T>(string json)
    {
        ThrowIfError(json);
        return JsonSerializer.Deserialize<T>(json, Opts)
               ?? throw new InvalidOperationException("empty response");
    }

    private static void ThrowIfError(string json)
    {
        using var doc = JsonDocument.Parse(json);
        if (doc.RootElement.TryGetProperty("error", out var e))
            throw new AevoraException(e.GetString() ?? "unknown error");
    }

    private static readonly JsonSerializerOptions Opts = new() { PropertyNameCaseInsensitive = true };

    public void Dispose()
    {
        if (_handle != IntPtr.Zero) { aevora_client_free(_handle); _handle = IntPtr.Zero; }
    }

    // --- native declarations ---
    [DllImport(Dll)] private static extern IntPtr aevora_client_new([MarshalAs(UnmanagedType.LPUTF8Str)] string baseUrl);
    [DllImport(Dll)] private static extern void aevora_client_free(IntPtr h);
    [DllImport(Dll)] private static extern void aevora_string_free(IntPtr s);
    [DllImport(Dll)] private static extern IntPtr aevora_enroll(IntPtr h, [MarshalAs(UnmanagedType.LPUTF8Str)] string invite, [MarshalAs(UnmanagedType.LPUTF8Str)] string email, [MarshalAs(UnmanagedType.LPUTF8Str)] string deviceName);
    [DllImport(Dll)] private static extern IntPtr aevora_restore(IntPtr h, [MarshalAs(UnmanagedType.LPUTF8Str)] string sessionJson);
    [DllImport(Dll)] private static extern IntPtr aevora_locations(IntPtr h);
    [DllImport(Dll)] private static extern IntPtr aevora_prepare_connection(IntPtr h, [MarshalAs(UnmanagedType.LPUTF8Str)] string country);
    [DllImport(Dll)] private static extern void aevora_mark_connected(IntPtr h);
    [DllImport(Dll)] private static extern IntPtr aevora_disconnect(IntPtr h);
    [DllImport(Dll)] private static extern IntPtr aevora_report_stats(IntPtr h, ulong rxBytes, ulong txBytes);
    [DllImport(Dll)] private static extern IntPtr aevora_current_stats(IntPtr h);
    [DllImport(Dll)] private static extern IntPtr aevora_state(IntPtr h);
}

public class AevoraException : Exception
{
    public AevoraException(string message) : base(message) { }
}

// --- DTOs (match the core's JSON) ---

public record Session
{
    public string device_id { get; init; } = "";
    public string user_id { get; init; } = "";
    public string refresh_token { get; init; } = "";
    public string private_key { get; init; } = "";
    public string public_key { get; init; } = "";
}

public record Location
{
    public string Code { get; init; } = "";
    public string Country { get; init; } = "";
    public bool Available { get; init; }
    public long Servers { get; init; }
}
public record LocationsResponse { public Location[] Locations { get; init; } = Array.Empty<Location>(); }

public record TunnelConfig
{
    public string Private_key { get; init; } = "";
    public string[] Addresses { get; init; } = Array.Empty<string>();
    public string[] Dns { get; init; } = Array.Empty<string>();
    public string Peer_public_key { get; init; } = "";
    public string Endpoint { get; init; } = "";
    public string[] Allowed_ips { get; init; } = Array.Empty<string>();
    public int Persistent_keepalive { get; init; }
}

public record Connection
{
    public string Connection_id { get; init; } = "";
    public string Server_name { get; init; } = "";
    public string Country { get; init; } = "";
    public string City { get; init; } = "";
    public string Endpoint { get; init; } = "";
    public string Assigned_ip { get; init; } = "";
    public TunnelConfig Config { get; init; } = new();
}

public record Stats
{
    public ulong Download_bps { get; init; }
    public ulong Upload_bps { get; init; }
    public uint Latency_ms { get; init; }
    public ulong Duration_seconds { get; init; }
}

public record StateResponse { public string State { get; init; } = ""; }
