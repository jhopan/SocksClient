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
        versionCode = 2
        versionName = "1.1.0"
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
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
