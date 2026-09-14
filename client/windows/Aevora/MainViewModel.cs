using System;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.IO;
using System.Net.Http;
using System.Runtime.CompilerServices;
using System.Threading.Tasks;
using System.Windows.Threading;

namespace Aevora;

public enum Phase { NeedsEnrollment, Disconnected, Connecting, Connected, Failed }



// MVVM view model. All auth/API/selection/lease/key/stats logic is in the shared
// Rust core (AevoraCore); this class orchestrates, runs the real WireGuardNT
// tunnel via TunnelService, and formats for the UI. No simulated state or stats.
public sealed class MainViewModel : INotifyPropertyChanged
{
    private readonly AevoraCore _core;
    private readonly DispatcherTimer _statsTimer;
    private static readonly HttpClient _http = new() { Timeout = TimeSpan.FromSeconds(5) };
    private const string DirectServiceName = "AevoraTunnel";
    private const string StatsLogPath = @"C:\AevoraDebug\stats-debug.log";

    public ObservableCollection<Location> Locations { get; } = new();

    private Phase _phase = Phase.NeedsEnrollment;
    public Phase Phase { get => _phase; private set { _phase = value; Raise(); Raise(nameof(StateText)); Raise(nameof(IsEnrolled)); Raise(nameof(IsConnected)); } }
    public bool IsDirectMode { get; private set; }

    public string StateText => Phase switch
    {
        Phase.Connected => "CONNECTED",
        Phase.Connecting => "CONNECTING…",
        Phase.Failed => "FAILED",
        _ => "DISCONNECTED",
    };
    public bool IsEnrolled => Phase != Phase.NeedsEnrollment;
    public bool IsConnected => Phase == Phase.Connected;

    private Location? _selected;
    public Location? SelectedCountry { get => _selected; set { _selected = value; Raise(); } }

    private string _serverName = "";
    public string ServerName { get => _serverName; private set { _serverName = value; Raise(); } }

    private string _duration = "00:00:00", _latency = "—", _download = "—", _upload = "—";
    public string Duration { get => _duration; private set { _duration = value; Raise(); } }
    public string Latency { get => _latency; private set { _latency = value; Raise(); } }
    public string Download { get => _download; private set { _download = value; Raise(); } }
    public string Upload { get => _upload; private set { _upload = value; Raise(); } }
    private DateTime? _connectedAt;

    private string? _error;
    public string? Error { get => _error; private set { _error = value; Raise(); } }

    private string _publicIp = "Fetching…";
    public string PublicIp { get => _publicIp; private set { _publicIp = value; Raise(); } }

    public MainViewModel(string controlUrl)
    {
        _core = new AevoraCore(controlUrl);
        _statsTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(3) };
        _statsTimer.Tick += (_, _) => UpdateStats();

        IsDirectMode = File.Exists(AppConfig.DirectConfigPath);
        Raise(nameof(IsDirectMode));
        WriteStatsLog($"[STATS] ViewModel initialized. BaseDirectory={AppContext.BaseDirectory}");

        if (IsDirectMode)
        {
            Phase = Phase.Disconnected;
        }
        else
        {
            var restored = SecureStore.Load();
            if (restored is not null)
            {
                _core.Restore(restored);
                Phase = Phase.Disconnected;
                _ = LoadLocationsAsync();
            }
        }
        _ = RefreshPublicIpAsync();
    }

    public async Task RefreshPublicIpAsync()
    {
        try
        {
            string ip = await _http.GetStringAsync("https://api.ipify.org");
            PublicIp = ip.Trim();
        }
        catch { PublicIp = "unavailable"; }
    }

    public async Task EnrollAsync(string invite, string email)
    {
        await Guarded(async () =>
        {
            var session = await Task.Run(() => _core.Enroll(invite, email, Environment.MachineName));
            SecureStore.Save(session);
            Phase = Phase.Disconnected;
            await LoadLocationsAsync();
        });
    }

    public async Task LoadLocationsAsync()
    {
        await Guarded(async () =>
        {
            var locs = await Task.Run(() => _core.Locations());
            Locations.Clear();
            foreach (var l in locs) Locations.Add(l);
        });
    }

    public async Task ConnectAsync()
    {
        if (IsDirectMode)
        {
            await ConnectDirectAsync();
            return;
        }

        var country = SelectedCountry?.Code;
        if (country is null) return;
        Phase = Phase.Connecting; Error = null;
        try
        {
            var conn = await Task.Run(() => _core.PrepareConnection(country)); // select + lease
            await Task.Run(() => TunnelService.Connect(conn.Config));           // real WireGuardNT tunnel
            _core.MarkConnected();
            ServerName = conn.Server_name;
            Phase = Phase.Connected;
            _statsTimer.Start();
            _ = RefreshPublicIpAsync();
        }
        catch (Exception e)
        {
            Phase = Phase.Failed; Error = e.Message;
            try { TunnelService.Disconnect(); } catch { }
            _ = RefreshPublicIpAsync();
        }
    }

    private async Task ConnectDirectAsync()
    {
        Phase = Phase.Connecting; Error = null;
        try
        {
            await Task.Run(() => ServiceManager.InstallAndStart(
                DirectServiceName,
                $"\"{Environment.ProcessPath} /service {AppConfig.DirectConfigPath}\""));
            ServerName = "Direct connection";
            Phase = Phase.Connected;
            _connectedAt = DateTime.UtcNow;
            _statsTimer.Start();
            WriteStatsLog("[STATS] Direct Mode connected; stats timer started.");
            UpdateStats();
            _ = RefreshPublicIpAsync();
        }
        catch (Exception e)
        {
            Phase = Phase.Failed; Error = e.Message;
            try { ServiceManager.StopAndRemove(DirectServiceName); } catch { }
            _ = RefreshPublicIpAsync();
        }
    }

    public async Task DisconnectAsync()
    {
        _statsTimer.Stop();
        WriteStatsLog("[STATS] Disconnected; stats timer stopped.");
        await Task.Run(() =>
        {
            if (IsDirectMode)
            {
                try { ServiceManager.StopAndRemove(DirectServiceName); } catch { }
            }
            else
            {
                try { TunnelService.Disconnect(); } catch { }
                try { _core.Disconnect(); } catch { }
            }
        });
        ServerName = "";
        Phase = Phase.Disconnected;
        _connectedAt = null;
        Duration = "00:00:00"; Latency = "—"; Download = "—"; Upload = "—";
        _ = RefreshPublicIpAsync();
    }

    private void UpdateStats()
    {
        if (Phase != Phase.Connected)
        {
            _statsTimer.Stop();
            WriteStatsLog("[STATS] Timer tick while disconnected; timer stopped.");
            return;
        }

        if (IsDirectMode)
        {
            DateTime connectedAt = _connectedAt ?? DateTime.UtcNow;
            Duration = FormatDuration((ulong)Math.Max(0, (DateTime.UtcNow - connectedAt).TotalSeconds));
            Latency = "—";

            Task.Run(() =>
            {
                try
                {
                    WriteStatsLog($"[STATS] Calling TryGetTransfer at {DateTime.Now:O}");
                    foreach (var adapterName in new[] { "real", "AevoraTunnel", "WireGuard" })
                    {
                        WriteStatsLog($"[STATS] Checking adapter: {adapterName}");
                    }

                    bool available = TunnelService.TryGetTransfer(out ulong rx, out ulong tx);
                    if (available)
                    {
                        WriteStatsLog($"[STATS] SUCCESS: rx={rx}, tx={tx}");
                    }
                    else
                    {
                        WriteStatsLog("[STATS] FAILED: TryGetTransfer returned false");
                    }

                    return (available, rx, tx);
                }
                catch (Exception ex)
                {
                    WriteStatsLog($"[STATS] EXCEPTION: {ex}");
                    throw;
                }
            }).ContinueWith(t =>
            {
                if (!t.IsCompletedSuccessfully || Phase != Phase.Connected || !IsDirectMode) return;
                if (t.Result.available)
                {
                    Download = FormatBytes(t.Result.rx);
                    Upload = FormatBytes(t.Result.tx);
                }
                else
                {
                    Download = "—";
                    Upload = "—";
                }
            }, TaskScheduler.FromCurrentSynchronizationContext());
            return;
        }

        Task.Run(() =>
        {
            ulong rx = 0, tx = 0;
            Stats stats;
            if (TunnelService.TryGetTransfer(out rx, out tx))
                stats = _core.ReportStats(rx, tx); // core computes rates + measures latency
            else
                stats = _core.CurrentStats();
            return stats;
        }).ContinueWith(t =>
        {
            if (!t.IsCompletedSuccessfully) return;
            var s = t.Result;
            Duration = FormatDuration(s.Duration_seconds);
            Download = FormatRate(s.Download_bps);
            Upload = FormatRate(s.Upload_bps);
            Latency = s.Latency_ms > 0 ? $"{s.Latency_ms} ms" : "—";
        }, TaskScheduler.FromCurrentSynchronizationContext());
    }

    private async Task Guarded(Func<Task> action)
    {
        try { Error = null; await action(); }
        catch (Exception e) { Error = e.Message; if (Phase == Phase.Connecting) Phase = Phase.Failed; }
    }

    private static void WriteStatsLog(string message)
    {
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(StatsLogPath)!);
            File.AppendAllText(StatsLogPath, $"{DateTime.Now:O} {message}{Environment.NewLine}");
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine(
                $"[STATS] LOGGING FAILED ({StatsLogPath}): {ex}");
        }
    }

    public static string FormatRate(ulong bytesPerSec)
    {
        double mbps = bytesPerSec * 8.0 / 1_000_000;
        return mbps >= 1 ? $"{mbps:F1} Mbps" : $"{bytesPerSec / 1024.0:F0} KB/s";
    }

    public static string FormatBytes(ulong bytes)
    {
        if (bytes == 0) return "0 B";
        string[] units = { "B", "KB", "MB", "GB", "TB" };
        double value = bytes;
        int unit = 0;
        while (value >= 1024 && unit < units.Length - 1)
        {
            value /= 1024;
            unit++;
        }
        return $"{value:0.0} {units[unit]}";
    }

    public static string FormatDuration(ulong seconds)
    {
        long s = (long)seconds;
        return $"{s / 3600:D2}:{(s % 3600) / 60:D2}:{s % 60:D2}";
    }

    public event PropertyChangedEventHandler? PropertyChanged;
    private void Raise([CallerMemberName] string? name = null) =>
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}
