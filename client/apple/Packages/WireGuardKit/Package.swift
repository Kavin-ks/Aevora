// swift-tools-version:5.9
// Local fork of wireguard-apple with two fixes:
//   1. swift-tools-version bumped to 5.9 (5.3 manifest rejects .v12/.v15 under Xcode 26.x)
//   2. WireGuardKitGo is a source target; pre-built libwg-go.a lives in prebuilt/<platform>/
//      and is found via LIBRARY_SEARCH_PATHS set on the depending Xcode target.
//
// Rebuild libwg-go.a:  cd client/apple && ./build-wg-go.sh
// That script places libwg-go.a in prebuilt/{macos,ios,iossim}/ automatically.

import PackageDescription

let package = Package(
    name: "WireGuardKit",
    platforms: [
        .macOS(.v14),
        .iOS(.v17)
    ],
    products: [
        .library(name: "WireGuardKit", targets: ["WireGuardKit"])
    ],
    targets: [
        .target(
            name: "WireGuardKit",
            dependencies: ["WireGuardKitGo", "WireGuardKitC"]
        ),
        .target(
            name: "WireGuardKitC",
            publicHeadersPath: "."
        ),
        // Thin C wrapper that exposes wireguard.h to Swift.
        // The real implementation (wgTurnOn, wgSetConfig, …) is in libwg-go.a,
        // which the *Xcode target* must add to LIBRARY_SEARCH_PATHS.
        .target(
            name: "WireGuardKitGo",
            exclude: [
                "goruntime-boottime-over-monotonic.diff",
                "go.mod", "go.sum", "api-apple.go", "Makefile"
            ],
            publicHeadersPath: ".",
            linkerSettings: [
                .linkedLibrary("wg-go")
            ]
        )
    ]
)
