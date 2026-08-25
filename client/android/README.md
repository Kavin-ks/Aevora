# Aevora — Android

Native Android app around `aevora-core`. The shared Rust core (via UniFFI Kotlin
bindings) owns all API/auth/session/selection/key/lease logic; the real tunnel is
carried by **wireguard-android**'s `GoBackend` over Android's `VpnService`.

> **Cannot be built in the repo's core loop.** Requires **Android Studio**, the
> **Android NDK**, `cargo-ndk`, and a device/emulator. The tunnel is real (GoBackend
> + `VpnService`); it is not mocked.

## What's here

```
android/
  build-core.sh                     Builds JNI libs + Kotlin bindings (run first)
  settings.gradle.kts / build.gradle.kts / app/build.gradle.kts
  app/src/main/AndroidManifest.xml  Declares GoBackend's VpnService
  app/src/main/kotlin/com/aevora/vpn/
    AevoraTunnelManager.kt          Core + GoBackend integration (the real bridge)
    MainActivity.kt                 Minimal launcher (VpnService consent → connect)
  app/src/main/jniLibs/             Generated (git-ignored): libaevora_core.so per ABI
  app/src/main/java/uniffi/         Generated (git-ignored): Kotlin bindings
```

## Build & run

```bash
cargo install cargo-ndk
rustup target add aarch64-linux-android armv7-linux-androideabi \
                  x86_64-linux-android i686-linux-android
export ANDROID_NDK_HOME=/path/to/ndk

cd client/android
./build-core.sh                 # build JNI libs + generate Kotlin bindings
# open in Android Studio, or:
./gradlew assembleDebug -PaevoraControlUrl=https://staging.control.example.com
```

Set the control-plane URL via `-PaevoraControlUrl=...` (or `local.properties`);
it is exposed as `BuildConfig.CONTROL_URL` and never hardcoded. As on Apple, you
need a running control plane with a real WireGuard gateway + node agent, and an
invite code.

The first connect triggers Android's system VPN consent dialog (`VpnService.prepare`).

## Status

The tunnel integration and core bridge are complete; the **UI is a stub**
(`MainActivity` exposes `connectTo(country)` / `disconnect()`), to be built into
the full consumer experience (map, locations, live stats) later. No fake tunnel
is presented — connecting establishes a genuine WireGuard `VpnService`.

## Security

- The device private key is generated in the core and, for hardening, should be
  stored in the Android Keystore / EncryptedSharedPreferences (a follow-up; the
  core currently returns it in `FfiSession` for the app to persist). Only the
  public key is sent to the control plane.
- No signing keystore, credentials, or endpoints are committed.
