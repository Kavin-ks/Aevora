//! Thin wrapper so `cargo run --features ffi --bin uniffi-bindgen` can generate
//! the Swift/Kotlin bindings from the built library.
fn main() {
    uniffi::uniffi_bindgen_main()
}
