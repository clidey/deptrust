using System;
using System.Linq;
using System.Text.Json;
using NuGet.Configuration;

// Test oracle uses the NuGet assemblies shipped with the installed SDK.
// It does not restore packages, run project code, or print source credentials.
class Program {
    static void Main(string[] args) {
        var settings = Settings.LoadSettingsGivenConfigPaths(args);
        var mapping = PackageSourceMapping.GetPackageSourceMapping(settings);
        var sources = new PackageSourceProvider(settings).LoadPackageSources()
            .Where(source => source.IsEnabled).ToArray();
        var names = mapping.IsEnabled
            ? mapping.GetConfiguredPackageSources("Company.Secret")
            : sources.Select(source => source.Name).ToArray();
        var selected = sources.Where(source => names.Contains(source.Name)).ToArray();
        Console.WriteLine(JsonSerializer.Serialize(new {
            isPublic = selected.Length > 0 && selected.All(source =>
                source.Source.TrimEnd('/') == "https://api.nuget.org/v3/index.json")
        }));
    }
}
