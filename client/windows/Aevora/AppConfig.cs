using System;
using System.IO;
using System.Text.Json;

namespace Aevora;

// Control-plane URL resolution order:
//   1. AEVORA_CONTROL_URL environment variable (set by user or IT)
//   2. appsettings.json next to the exe (baked in by CI build for pre-configured installs)
//   3. Empty string (user sees a helpful error in the enrollment screen)
internal static class AppConfig
{
    private static string? _cached;

    public static string ControlUrl => _cached ??= Resolve();

    private static string Resolve()
    {
        // 1. Environment variable wins (allows override without rebuild)
        var env = Environment.GetEnvironmentVariable("AEVORA_CONTROL_URL")?.Trim();
        if (!string.IsNullOrEmpty(env)) return env;

        // 2. appsettings.json next to the executable
        try
        {
            var dir = AppContext.BaseDirectory;
            var path = Path.Combine(dir, "appsettings.json");
            if (File.Exists(path))
            {
                var doc = JsonDocument.Parse(File.ReadAllText(path));
                if (doc.RootElement.TryGetProperty("ControlUrl", out var el))
                {
                    var v = el.GetString()?.Trim();
                    if (!string.IsNullOrEmpty(v)) return v;
                }
            }
        }
        catch { /* ignore malformed settings */ }

        return "";
    }
}
