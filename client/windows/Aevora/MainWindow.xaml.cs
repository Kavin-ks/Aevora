using System.Collections.Generic;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Shapes;

namespace Aevora;

public partial class MainWindow : Window
{
    private readonly MainViewModel _vm;

    // Static coordinates for map placement only (availability comes from the core).
    private static readonly Dictionary<string, (double lat, double lon)> Geo = new()
    {
        ["de"] = (50.11, 8.68), ["nl"] = (52.37, 4.90), ["gb"] = (51.51, -0.13),
        ["fr"] = (48.85, 2.35), ["us"] = (40.71, -74.01), ["ca"] = (43.65, -79.38),
        ["sg"] = (1.35, 103.82), ["jp"] = (35.68, 139.69), ["in"] = (19.08, 72.88),
        ["au"] = (-33.87, 151.21), ["br"] = (-23.55, -46.63), ["ae"] = (25.20, 55.27),
    };

    public MainWindow()
    {
        InitializeComponent();
        _vm = new MainViewModel(AppConfig.ControlUrl);
        DataContext = _vm;

        _vm.PropertyChanged += (_, e) => { if (e.PropertyName == nameof(MainViewModel.Phase)) UpdateButton(); };
        _vm.Locations.CollectionChanged += (_, _) => DrawMap();
        MapCanvas.SizeChanged += (_, _) => DrawMap();
        UpdateButton();
    }

    private void UpdateButton()
    {
        ConnectButton.Content = _vm.Phase switch
        {
            Phase.Connected => "Disconnect",
            Phase.Connecting => "Cancel",
            _ => "Connect",
        };
        ConnectButton.Background = _vm.Phase == Phase.Connected
            ? Brushes.IndianRed
            : (Brush)FindResource("AevoraTeal");
    }

    private async void OnEnroll(object sender, RoutedEventArgs e)
        => await _vm.EnrollAsync(InviteBox.Text.Trim(), EmailBox.Text.Trim());

    private async void OnConnectToggle(object sender, RoutedEventArgs e)
    {
        if (_vm.Phase is Phase.Connected or Phase.Connecting) await _vm.DisconnectAsync();
        else await _vm.ConnectAsync();
    }

    private void OnLocationSelected(object sender, SelectionChangedEventArgs e)
        => _vm.SelectedCountry = LocationsList.SelectedItem as Location;

    private void DrawMap()
    {
        MapCanvas.Children.Clear();
        double w = MapCanvas.ActualWidth <= 0 ? 400 : MapCanvas.ActualWidth;
        double h = MapCanvas.ActualHeight <= 0 ? 200 : MapCanvas.ActualHeight;

        foreach (var loc in _vm.Locations)
        {
            if (!Geo.TryGetValue(loc.Code.ToLowerInvariant(), out var g)) continue;
            double x = (g.lon + 180) / 360 * w;
            double y = (90 - g.lat) / 180 * h;
            var dot = new Ellipse
            {
                Width = 11,
                Height = 11,
                Fill = loc.Available ? (Brush)FindResource("AevoraTeal") : Brushes.Gray,
            };
            Canvas.SetLeft(dot, x - 5.5);
            Canvas.SetTop(dot, y - 5.5);
            var captured = loc;
            dot.MouseLeftButtonUp += (_, _) => { if (captured.Available) LocationsList.SelectedItem = captured; };
            MapCanvas.Children.Add(dot);
        }
    }
}
