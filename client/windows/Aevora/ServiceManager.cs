using System;
using System.Diagnostics;
using System.ServiceProcess;
using System.Threading;

namespace Aevora;

// Installs/starts/stops the tunnel Windows service via sc.exe. The app runs
// elevated (see app.manifest), so no per-call UAC prompt. The service's binary
// path points back at this exe with "/service <conf>", whose entry point calls
// TunnelService.RunServiceEntryPoint -> WireGuardTunnelService.
internal static class ServiceManager
{
    public static void InstallAndStart(string name, string binPath)
    {
        // Remove any stale instance first.
        StopAndRemove(name);
        Sc($"create {name} binPath= {binPath} type= own start= demand");
        Sc($"start {name}");
        WaitForStatus(name, ServiceControllerStatus.Running, TimeSpan.FromSeconds(15));
    }

    public static void StopAndRemove(string name)
    {
        if (!Exists(name)) return;
        try
        {
            using var sc = new ServiceController(name);
            if (sc.Status != ServiceControllerStatus.Stopped)
            {
                sc.Stop();
                sc.WaitForStatus(ServiceControllerStatus.Stopped, TimeSpan.FromSeconds(15));
            }
        }
        catch { /* ignore */ }
        Sc($"delete {name}");
    }

    private static bool Exists(string name)
    {
        foreach (var s in ServiceController.GetServices())
            if (string.Equals(s.ServiceName, name, StringComparison.OrdinalIgnoreCase))
                return true;
        return false;
    }

    private static void WaitForStatus(string name, ServiceControllerStatus status, TimeSpan timeout)
    {
        using var sc = new ServiceController(name);
        sc.WaitForStatus(status, timeout);
    }

    private static void Sc(string args)
    {
        var psi = new ProcessStartInfo("sc.exe", args)
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        using var p = Process.Start(psi)!;
        p.WaitForExit();
        Thread.Sleep(200); // let the SCM settle between operations
    }
}
