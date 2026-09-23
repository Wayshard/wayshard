//! Platform-secure device-credential storage for the native (Desktop/Android)
//! clients.
//!
//! Desktop targets use the OS credential store (macOS Keychain, Windows
//! Credential Manager, Linux Secret Service) through the `keyring` crate.
//! Android uses an Android Keystore AES-256-GCM key that is non-exportable: only
//! ciphertext plus metadata is written to app-private storage
//! (`credential_store`), and the Kotlin helper performs the Keystore operations
//! (`android_keystore`).

const KEYRING_SERVICE: &str = "dev.wayshard.app";
const KEYRING_USER: &str = "device-credential";

#[cfg(not(target_os = "android"))]
fn keyring_entry() -> Result<keyring::Entry, String> {
    keyring::Entry::new(KEYRING_SERVICE, KEYRING_USER).map_err(|e| e.to_string())
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
pub fn save_credential(credential: String) -> Result<(), String> {
    keyring_entry()?
        .set_password(&credential)
        .map_err(|e| e.to_string())
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
pub fn load_credential() -> Result<String, String> {
    match keyring_entry()?.get_password() {
        Ok(v) => Ok(v),
        Err(keyring::Error::NoEntry) => Ok(String::new()),
        Err(e) => Err(e.to_string()),
    }
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
pub fn clear_credential() -> Result<(), String> {
    match keyring_entry()?.delete_credential() {
        Ok(()) => Ok(()),
        Err(keyring::Error::NoEntry) => Ok(()),
        Err(e) => Err(e.to_string()),
    }
}

#[cfg(target_os = "android")]
fn credential_path(app: &tauri::AppHandle) -> Result<std::path::PathBuf, String> {
    use tauri::Manager;
    let dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    Ok(dir.join("credential.json"))
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn save_credential(app: tauri::AppHandle, credential: String) -> Result<(), String> {
    crate::credential_store::save_encrypted(
        &credential_path(&app)?,
        &crate::android_keystore::KeystoreCipher,
        &credential,
    )
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn load_credential(app: tauri::AppHandle) -> Result<String, String> {
    Ok(crate::credential_store::load_encrypted(
        &credential_path(&app)?,
        &crate::android_keystore::KeystoreCipher,
    )?
    .unwrap_or_default())
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn clear_credential(app: tauri::AppHandle) -> Result<(), String> {
    let path = credential_path(&app)?;
    let removed = crate::credential_store::clear_file(&path);
    if removed.is_ok() {
        // Best-effort: drop the Keystore key too so key material does not
        // linger. Only done once the ciphertext is gone, so a failure here can
        // never strand undecryptable data.
        let _ = crate::android_keystore::invoke("deleteKey", "");
    }
    removed
}
