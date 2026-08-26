import SwiftUI
import MapKit
import aevora_core

// The world map. Availability and the country list come from the control plane
// (model.locations); only the on-map POSITION uses a static coordinate lookup,
// so which countries are "available" is never hardcoded. Requires macOS 14 /
// iOS 17 for the SwiftUI Map content builder.

struct WorldMapView: View {
    @EnvironmentObject var model: AppModel
    @State private var camera: MapCameraPosition = .automatic

    var body: some View {
        Map(position: $camera) {
            ForEach(model.locations, id: \.code) { loc in
                if let coord = CountryCoordinates.lookup(loc.code) {
                    Annotation(loc.country, coordinate: coord) {
                        LocationPin(
                            available: loc.available,
                            selected: model.selectedCountry == loc.code
                        )
                        .onTapGesture { if loc.available { model.selectedCountry = loc.code } }
                        .help(loc.available ? loc.country : "\(loc.country) — unavailable")
                    }
                }
            }
        }
        .mapStyle(.standard(elevation: .flat))
        .frame(minHeight: 300)
        .clipShape(RoundedRectangle(cornerRadius: 14))
    }
}

private struct LocationPin: View {
    let available: Bool
    let selected: Bool

    var body: some View {
        ZStack {
            Circle()
                .fill(available ? Theme.accent : Color.secondary.opacity(0.5))
                .frame(width: selected ? 20 : 13, height: selected ? 20 : 13)
            if selected {
                Circle().stroke(Theme.accent, lineWidth: 3).frame(width: 30, height: 30)
            }
        }
        .shadow(radius: 2)
        .animation(.easeInOut(duration: 0.15), value: selected)
    }
}

/// Representative coordinates (capital-ish) for placing a country's marker.
/// Presentation only — it does not define availability. Unknown codes are simply
/// not drawn on the map (they still appear in the list).
enum CountryCoordinates {
    static func lookup(_ code: String) -> CLLocationCoordinate2D? {
        table[code.lowercased()]
    }

    private static let table: [String: CLLocationCoordinate2D] = [
        "de": .init(latitude: 50.11, longitude: 8.68),   // Frankfurt
        "nl": .init(latitude: 52.37, longitude: 4.90),   // Amsterdam
        "gb": .init(latitude: 51.51, longitude: -0.13),  // London
        "fr": .init(latitude: 48.85, longitude: 2.35),   // Paris
        "us": .init(latitude: 40.71, longitude: -74.01), // New York
        "ca": .init(latitude: 43.65, longitude: -79.38), // Toronto
        "sg": .init(latitude: 1.35, longitude: 103.82),  // Singapore
        "jp": .init(latitude: 35.68, longitude: 139.69), // Tokyo
        "in": .init(latitude: 19.08, longitude: 72.88),  // Mumbai
        "au": .init(latitude: -33.87, longitude: 151.21),// Sydney
        "br": .init(latitude: -23.55, longitude: -46.63),// São Paulo
        "ae": .init(latitude: 25.20, longitude: 55.27),  // Dubai
    ]
}

/// Original Aevora visual identity (minimal, no third-party branding).
enum Theme {
    static let accent = Color(red: 0.05, green: 0.62, blue: 0.56) // Aevora teal
}
