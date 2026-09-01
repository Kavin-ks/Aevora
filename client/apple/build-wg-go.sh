#!/usr/bin/env bash
#
# Builds the WireGuard Go library (libwg-go.a) for all Apple targets and
# assembles WireGuardKitGo.xcframework used by our local WireGuardKit package.
#
# Requirements:
#   - Full Xcode installed and selected (xcode-select -s /Applications/Xcode.app/...)
#   - Go 1.21+ (checks $HOME/.aevora-go/go/bin/go, then $PATH)
#
# Run once before opening Xcode (build-core.sh calls this automatically):
#   cd client/apple && ./build-wg-go.sh
#
# Output (tracked in git — regenerate if you update the WireGuard Go source):
#   Packages/WireGuardKit/WireGuardKitGo.xcframework

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PKG="$HERE/Packages/WireGuardKit"
SRC="$HERE/wireguard-go-src"   # temp extraction directory

# Locate Go — prefer the project-local install
if command -v go &>/dev/null && go env GOVERSION &>/dev/null; then
    GO="$(command -v go)"
elif [[ -x "$HOME/.aevora-go/go/bin/go" ]]; then
    GO="$HOME/.aevora-go/go/bin/go"
else
    echo "ERROR: go not found. Install Go 1.21+ or set \$HOME/.aevora-go/go/bin/go" >&2
    exit 1
fi
export GOTOOLCHAIN=local
echo "==> Using Go: $($GO version)"

# Locate the WireGuardKitGo source (from SPM cache or wireguard-apple repo)
REPO=$(ls -d ~/Library/Developer/Xcode/DerivedData/Aevora-*/SourcePackages/repositories/wireguard-apple-* 2>/dev/null | head -1 || true)
if [[ -z "$REPO" ]]; then
    echo "ERROR: wireguard-apple git cache not found. Open the Xcode project once" >&2
    echo "       to let SPM fetch WireGuard/wireguard-apple, then re-run this script." >&2
    exit 1
fi

rm -rf "$SRC"
mkdir -p "$SRC"
git --git-dir="$REPO" archive HEAD -- Sources/WireGuardKitGo | tar -x --strip-components=2 -C "$SRC"

echo "==> Building libwg-go.a for all Apple targets"
BUILD="$SRC/.build"
mkdir -p "$BUILD"

# --- macOS (arm64 + x86_64) ---
MACSDKROOT=$(xcrun --sdk macosx --show-sdk-path)
for ARCH in arm64 x86_64; do
    CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" \
        CC="$(xcrun --sdk macosx -f clang)" \
        CGO_CFLAGS="-isysroot $MACSDKROOT -arch $ARCH" \
        CGO_LDFLAGS="-isysroot $MACSDKROOT -arch $ARCH" \
        "$GO" build -ldflags=-w -trimpath \
        -o "$BUILD/macos-$ARCH.a" -buildmode c-archive "$SRC"
done
lipo -create "$BUILD/macos-arm64.a" "$BUILD/macos-x86_64.a" -output "$BUILD/macos.a"

# --- iOS device (arm64) ---
IOSSDKROOT=$(xcrun --sdk iphoneos --show-sdk-path)
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
    CC="$(xcrun --sdk iphoneos -f clang)" \
    CGO_CFLAGS="-isysroot $IOSSDKROOT -mios-version-min=17.0 -arch arm64" \
    CGO_LDFLAGS="-isysroot $IOSSDKROOT -mios-version-min=17.0 -arch arm64" \
    "$GO" build -ldflags=-w -trimpath \
    -o "$BUILD/ios-arm64.a" -buildmode c-archive "$SRC"

# --- iOS simulator (arm64 + x86_64) ---
SIMSDKROOT=$(xcrun --sdk iphonesimulator --show-sdk-path)
for ARCH in arm64 x86_64; do
    CGO_ENABLED=1 GOOS=ios GOARCH="$([ "$ARCH" = x86_64 ] && echo amd64 || echo arm64)" \
        CC="$(xcrun --sdk iphonesimulator -f clang)" \
        CGO_CFLAGS="-isysroot $SIMSDKROOT -miphonesimulator-version-min=17.0 -arch $ARCH" \
        CGO_LDFLAGS="-isysroot $SIMSDKROOT -miphonesimulator-version-min=17.0 -arch $ARCH" \
        "$GO" build -ldflags=-w -trimpath \
        -o "$BUILD/iossim-$ARCH.a" -buildmode c-archive "$SRC"
done
lipo -create "$BUILD/iossim-arm64.a" "$BUILD/iossim-x86_64.a" -output "$BUILD/iossim.a"

echo "==> Assembling WireGuardKitGo.xcframework"
# Headers: wireguard.h + module.modulemap
HDRS="$BUILD/headers"
mkdir -p "$HDRS"
cat > "$HDRS/wireguard.h" << 'WIREGUARD_H'
/* SPDX-License-Identifier: MIT
 * Copyright (C) 2018-2023 WireGuard LLC. All Rights Reserved. */
#ifndef WIREGUARD_H
#define WIREGUARD_H
#include <sys/types.h>
#include <stdint.h>
#include <stdbool.h>
typedef void(*logger_fn_t)(void *context, int level, const char *msg);
extern void wgSetLogger(void *context, logger_fn_t logger_fn);
extern int wgTurnOn(const char *settings, int32_t tun_fd);
extern void wgTurnOff(int handle);
extern int64_t wgSetConfig(int handle, const char *settings);
extern char *wgGetConfig(int handle);
extern void wgBumpSockets(int handle);
extern void wgDisableSomeRoamingForBrokenMobileSemantics(int handle);
extern const char *wgVersion();
#endif
WIREGUARD_H
cat > "$HDRS/module.modulemap" << 'MODULE_MAP'
module WireGuardKitGo {
    umbrella header "wireguard.h"
    export *
}
MODULE_MAP

rm -rf "$PKG/WireGuardKitGo.xcframework"
xcodebuild -create-xcframework \
    -library "$BUILD/macos.a"   -headers "$HDRS" \
    -library "$BUILD/ios-arm64.a" -headers "$HDRS" \
    -library "$BUILD/iossim.a"  -headers "$HDRS" \
    -output "$PKG/WireGuardKitGo.xcframework"

rm -rf "$SRC"
echo
echo "  ✔ WireGuardKitGo.xcframework updated."
