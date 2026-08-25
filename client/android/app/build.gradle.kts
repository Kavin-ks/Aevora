plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.aevora.vpn"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.aevora.vpn"
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"
        ndk { abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86_64", "x86") }

        // Control-plane URL comes from a Gradle property (-PaevoraControlUrl=...)
        // or local.properties; never hardcoded. Read in code via BuildConfig.
        val controlUrl = (project.findProperty("aevoraControlUrl") as String?) ?: ""
        buildConfigField("String", "CONTROL_URL", "\"$controlUrl\"")
    }

    buildFeatures { buildConfig = true }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    // Official WireGuard Android backend (GoBackend + config parser).
    implementation("com.wireguard.android:tunnel:1.0.20230706")
    // UniFFI-generated Kotlin bindings use JNA at runtime.
    implementation("net.java.dev.jna:jna:5.14.0@aar")
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
}
