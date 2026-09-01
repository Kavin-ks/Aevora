#!/usr/bin/env bash
#
# Builds aevora-core into an AevoraCore.xcframework and generates the Swift
# UniFFI bindings, for the macOS + iOS apps to link.
#
# Run on macOS with **full Xcode** installed and selected:
#   sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
#   cd client/apple && ./build-core.sh
#
# Outputs (git-ignored):
#   build/AevoraCore.xcframework
#   build/Generated/aevora_core.swift
#
# The Xcode project references these; run this script before opening Xcode.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE="$(cd "$HERE/../core" && pwd)"
OUT="$HERE/build"
LIBDIR="$OUT/lib"
GEN="$OUT/Generated"
HEADERS="$OUT/headers"
NAME="aevora_core"

command -v cargo >/dev/null || { echo "cargo not found (install Rust)"; exit 1; }
xcodebuild -version >/dev/null 2>&1 || { echo "full Xcode required (xcode-select -s /Applications/Xcode.app/...)"; exit 1; }

# Which Apple targets to build. macOS is required; iOS is included for Phase 2b's
# shared codebase. Comment out the iOS lines to build macOS only.
MAC_TARGETS=(aarch64-apple-darwin x86_64-apple-darwin)
IOS_DEVICE=(aarch64-apple-ios)
IOS_SIM=(aarch64-apple-ios-sim x86_64-apple-ios)
ALL=("${MAC_TARGETS[@]}" "${IOS_DEVICE[@]}" "${IOS_SIM[@]}")

echo "==> Ensuring rustup targets"
for t in "${ALL[@]}"; do rustup target add "$t" >/dev/null 2>&1 || true; done

echo "==> Building the static library for each target (release, feature ffi)"
rm -rf "$OUT"; mkdir -p "$LIBDIR" "$GEN" "$HEADERS"
for t in "${ALL[@]}"; do
  ( cd "$CORE" && cargo build --release --features ffi --target "$t" )
done

lib() { echo "$CORE/target/$1/release/lib${NAME}.a"; }

echo "==> Fattening per-platform slices with lipo"
lipo -create $(for t in "${MAC_TARGETS[@]}"; do lib "$t"; done) -output "$LIBDIR/libcore-macos.a"
lipo -create $(for t in "${IOS_SIM[@]}";    do lib "$t"; done) -output "$LIBDIR/libcore-iossim.a"
cp "$(lib "${IOS_DEVICE[0]}")" "$LIBDIR/libcore-ios.a"

echo "==> Generating Swift bindings"
# --library mode reads metadata from a built dylib for the host.
( cd "$CORE" && cargo build --release --features ffi )
cargo run --manifest-path "$CORE/Cargo.toml" --release --features ffi --bin uniffi-bindgen -- \
  generate --library "$CORE/target/release/lib${NAME}.dylib" --language swift --out-dir "$GEN"

# The xcframework needs a headers dir with the C header + a module map.
cp "$GEN/${NAME}FFI.h" "$HEADERS/"
cp "$GEN/${NAME}FFI.modulemap" "$HEADERS/module.modulemap"

echo "==> Assembling AevoraCore.xcframework"
rm -rf "$OUT/AevoraCore.xcframework"
xcodebuild -create-xcframework \
  -library "$LIBDIR/libcore-macos.a"  -headers "$HEADERS" \
  -library "$LIBDIR/libcore-ios.a"    -headers "$HEADERS" \
  -library "$LIBDIR/libcore-iossim.a" -headers "$HEADERS" \
  -output "$OUT/AevoraCore.xcframework"

echo
echo "  ✔ Done."
echo "    build/AevoraCore.xcframework"
echo "    build/Generated/aevora_core.swift  (add to the app + extension targets)"
