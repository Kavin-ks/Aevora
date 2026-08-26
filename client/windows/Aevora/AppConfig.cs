using System;

namespace Aevora;

// Control-plane URL comes from the AEVORA_CONTROL_URL environment variable (or an
// appsettings override), never hardcoded, so dev/staging/prod are not baked in.
internal static class AppConfig
{
    public static string ControlUrl =>
        Environment.GetEnvironmentVariable("AEVORA_CONTROL_URL")?.Trim() ?? "";
}
