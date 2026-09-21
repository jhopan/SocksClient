plugins {
    id("com.android.application")
}

tasks.register("prepareKotlinBuildScriptModel")

dependencies {
    // Hanya libbox.aar (sing-box core) — zero external dependencies!
    implementation(files("libs/libbox.aar"))
}

android {
    namespace = "com.jhopanstore.socksclient"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.jhopanstore.socksclient"
        minSdk = 24
        targetSdk = 35
        // Konvensi: fitur baru -> naikkan versi normal (1.3.0).
        // Build ulang tanpa fitur baru -> tambah segmen keempat (1.3.0.1, 1.3.0.2, ...).
        // versionCode selalu naik: major*10000 + minor*100 + patch*10 + build.
        versionCode = 140000
        versionName = "1.4.0"
    }

    buildFeatures {
        buildConfig = true
    }

    // ── APK Splits: 3 variant (arm64-v8a, armeabi-v7a, universal) ──
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a")
            isUniversalApk = true
        }
    }

    // S1: APK rilis ditandatangani kunci rilis, bukan kunci debug.
    // Kunci debug bersifat publik dan APK yang ditandatangani dengannya tidak
    // bisa di-update setelah pindah ke kunci rilis (harus uninstall dulu).
    // Keystore diambil dari KEYSTORE_FILE (CI mendekode secret) - kalau tidak
    // ada, build lokal jatuh kembali ke kunci debug supaya tetap bisa dikompilasi.
    val releaseKeystore = System.getenv("KEYSTORE_FILE")?.let { file(it) }
    val hasReleaseKey = releaseKeystore?.exists() == true

    signingConfigs {
        if (hasReleaseKey) {
            create("release") {
                storeFile = releaseKeystore
                storePassword = System.getenv("KEYSTORE_PASSWORD")
                keyAlias = System.getenv("KEY_ALIAS")
                keyPassword = System.getenv("KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            signingConfig = if (hasReleaseKey) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
