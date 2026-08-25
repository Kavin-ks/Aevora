import SwiftUI

@main
struct AevoraApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            ContentView().environmentObject(model)
        }
        #if os(macOS)
        .windowResizability(.contentSize)
        #endif
    }
}
