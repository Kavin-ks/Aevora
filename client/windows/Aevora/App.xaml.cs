using System.Windows;

namespace Aevora;

public partial class App : Application
{
    protected override void OnStartup(StartupEventArgs e)
    {
        // When launched by the SCM as "Aevora.exe /service <conf>", run the
        // WireGuard tunnel loop instead of the UI.
        if (e.Args.Length >= 2 && e.Args[0] == "/service")
        {
            int code = TunnelService.RunServiceEntryPoint(e.Args[1]);
            Shutdown(code);
            return;
        }

        base.OnStartup(e);
        new MainWindow().Show();
    }
}
