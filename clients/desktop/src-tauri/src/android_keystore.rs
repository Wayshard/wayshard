//! Android Keystore-backed credential cipher.
//!
//! An AES-256-GCM key lives in the Android Keystore and is non-exportable; only
//! ciphertext is written to app-private storage. The Kotlin helper
//! `dev.wayshard.app.WayshardKeystore` performs the Keystore operations and this
//! module reaches it over JNI.
//!
//! The module is compiled under `test` as well so the JNI usage is type-checked
//! on the development host; it is only wired into the app on Android.

#![cfg(any(target_os = "android", test))]

use crate::credential_store::CredentialCipher;

const HELPER_CLASS: &str = "dev/wayshard/app/WayshardKeystore";

type GetCreatedJavaVMs = unsafe extern "C" fn(
    vm_buf: *mut *mut jni::sys::JavaVM,
    buf_len: jni::sys::jsize,
    n_vms: *mut jni::sys::jsize,
) -> jni::sys::jint;

/// java_vm resolves the already-created JavaVM. `JNI_GetCreatedJavaVMs` is
/// exported by the Android runtime and is resolved dynamically so the desktop
/// build never links a JVM.
fn java_vm() -> Result<jni::JavaVM, String> {
    let sym = unsafe {
        libc::dlsym(
            libc::RTLD_DEFAULT,
            b"JNI_GetCreatedJavaVMs\0".as_ptr() as *const libc::c_char,
        )
    };
    if sym.is_null() {
        return Err("JNI_GetCreatedJavaVMs is unavailable".to_string());
    }
    let get_vms: GetCreatedJavaVMs = unsafe { std::mem::transmute(sym) };
    let mut vm: *mut jni::sys::JavaVM = std::ptr::null_mut();
    let mut count: jni::sys::jsize = 0;
    let rc = unsafe { get_vms(&mut vm, 1, &mut count) };
    if rc != 0 || vm.is_null() || count < 1 {
        return Err("no JavaVM is available in this process".to_string());
    }
    unsafe { jni::JavaVM::from_raw(vm) }.map_err(|e| e.to_string())
}

/// invoke calls a static `(String) -> String` method on the Kotlin helper.
fn invoke(op: &str, input: &str) -> Result<String, String> {
    let vm = java_vm()?;
    let mut env = vm.attach_current_thread().map_err(|e| e.to_string())?;
    let class = env.find_class(HELPER_CLASS).map_err(|e| e.to_string())?;
    let arg = env.new_string(input).map_err(|e| e.to_string())?;
    let result = env
        .call_static_method(
            &class,
            op,
            "(Ljava/lang/String;)Ljava/lang/String;",
            &[jni::objects::JValue::Object(&arg)],
        )
        .map_err(|e| e.to_string())?;
    let obj = result.l().map_err(|e| e.to_string())?;
    if obj.is_null() {
        return Err(format!("keystore {op} returned null"));
    }
    let jstr: jni::objects::JString = obj.into();
    let out = env.get_string(&jstr).map_err(|e| e.to_string())?;
    Ok(out.to_string_lossy().into_owned())
}

/// KeystoreCipher encrypts with the Android Keystore AES-256-GCM key.
pub struct KeystoreCipher;

impl CredentialCipher for KeystoreCipher {
    fn name(&self) -> &'static str {
        "android-keystore-aes-gcm"
    }

    fn encrypt(&self, plaintext: &[u8]) -> Result<String, String> {
        let text =
            std::str::from_utf8(plaintext).map_err(|_| "credential is not utf-8".to_string())?;
        invoke("encrypt", text)
    }

    fn decrypt(&self, token: &str) -> Result<Vec<u8>, String> {
        Ok(invoke("decrypt", token)?.into_bytes())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // On the development host there is no JVM, so the binding must fail closed
    // rather than panic or silently succeed.
    #[test]
    fn missing_jvm_fails_closed() {
        assert!(java_vm().is_err());
        assert!(invoke("encrypt", "value").is_err());
    }

    #[test]
    fn cipher_name_is_stable() {
        assert_eq!(KeystoreCipher.name(), "android-keystore-aes-gcm");
    }
}
