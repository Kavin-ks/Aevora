#!/usr/bin/env bash
#
# Builds aevora-core for Android (JNI shared libs) and generates the Kotlin
# UniFFI bindings. Run on a machine with the Android NDK + cargo-ndk:
#
#   cargo install cargo-ndk
#   rustup target add aarch64-linux-android armv7-linux-androideabi \
#                     x86_64-linux-android i686-linux-android
#   export ANDROID_NDK_HOME=/path/to/ndk
#   cd client/android && ./build-core.sh
#
# Outputs (git-ignored):
#   app/src/main/jniLibs/<abi>/libaevora_core.so
#   app/src/main/java/uniffi/aevora_core/aevora_core.kt

set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE="$(cd "$HERE/../core" && pwd)"

command -v cargo-ndk >/dev/null || { echo "cargo-ndk not found (cargo install cargo-ndk)"; exit 1; }
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to your NDK path}"

echo "==> Building JNI libraries (release, feature ffi)"
( cd "$CORE" && cargo ndk \
    -t arm64-v8a -t armeabi-v7a -t x86_64 -t x86 \
    -o "$HERE/app/src/main/jniLibs" \
    build --release --features ffi )

echo "==> Generating Kotlin bindings"
GEN="$HERE/app/src/main/java"
mkdir -p "$GEN"
( cd "$CORE" && cargo build --release --features ffi )
cargo run --manifest-path "$CORE/Cargo.toml" --release --features ffi --bin uniffi-bindgen -- \
  generate --library "$CORE/target/release/libaevora_core.dylib" --language kotlin --out-dir "$GEN" \
  || cargo run --manifest-path "$CORE/Cargo.toml" --release --features ffi --bin uniffi-bindgen -- \
  generate --library "$CORE/target/release/libaevora_core.so" --language kotlin --out-dir "$GEN"

echo "  ✔ Done. JNI libs in app/src/main/jniLibs, bindings in app/src/main/java/uniffi."
