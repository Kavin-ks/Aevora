using System;
using System.Collections.ObjectModel;
using System.ComponentModel;
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

    public ObservableCollection<Location> Locations { get; } = new();

    private Phase _phase = Phase.NeedsEnrollment;
    public Phase Phase { get => _phase; private set { _phase = value; Raise(); Raise(nameof(StateText)); Raise(nameof(IsEnrolled)); Raise(nameof(IsConnected)); } }

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

    private string? _error;
    public string? Error { get => _error; private set { _error = value; Raise(); } }

    public MainViewModel(string controlUrl)
    {
        _core = new AevoraCore(controlUrl);
        _statsTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(3) };
        _statsTimer.Tick += (_, _) => UpdateStats();

        var restored = SecureStore.Load();
        if (restored is not null)
        {
            _core.Restore(restored);
            Phase = Phase.Disconnected;
            _ = LoadLocationsAsync();
        }
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
        }
        catch (Exception e)
        {
            Phase = Phase.Failed; Error = e.Message;
            try { TunnelService.Disconnect(); } catch { }
        }
    }

    public async Task DisconnectAsync()
    {
        _statsTimer.Stop();
        await Task.Run(() =>
        {
            try { TunnelService.Disconnect(); } catch { }
            try { _core.Disconnect(); } catch { }
        });
        ServerName = "";
        Phase = Phase.Disconnected;
        Duration = "00:00:00"; Latency = "—"; Download = "—"; Upload = "—";
    }

    private void UpdateStats()
    {
        if (Phase != Phase.Connected) return;
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

    public static string FormatRate(ulong bytesPerSec)
    {
        double mbps = bytesPerSec * 8.0 / 1_000_000;
        return mbps >= 1 ? $"{mbps:F1} Mbps" : $"{bytesPerSec / 1024.0:F0} KB/s";
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
