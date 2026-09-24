# R8/ProGuard keep rules for the Android Keystore credential helper.
#
# dev.wayshard.app.WayshardKeystore is reached ONLY from Rust over JNI
# (JNIEnv::find_class + call_static_method), so R8 cannot see a Java/Kotlin
# reference and the minified release build would otherwise strip the class or
# rename its encrypt/decrypt/deleteKey methods. JNI looks the class and methods
# up by their exact names, so both the class name and every member must survive.
#
# Tauri's generated app/build.gradle.kts release build type enables
# `isMinifyEnabled = true` and collects every `**/*.pro` under the app module,
# so this file is picked up automatically once installed beside the helper.
# The package placeholder is rewritten to the app identifier by
# scripts/release/android-keystore-patch.sh.
-keep class __PACKAGE__.WayshardKeystore { *; }
