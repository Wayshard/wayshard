//! Platform-secure device-credential storage for the native (Desktop/Android)
//! clients.
//!
//! Desktop targets use the OS credential store (macOS Keychain, Windows
//! Credential Manager, Linux Secret Service) through the `keyring` crate.
//! Android uses the app's private internal storage, which is sandboxed to the
//! application by the platform.

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
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    Ok(dir.join("device-credential"))
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn save_credential(app: tauri::AppHandle, credential: String) -> Result<(), String> {
    std::fs::write(credential_path(&app)?, credential).map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn load_credential(app: tauri::AppHandle) -> Result<String, String> {
    match std::fs::read_to_string(credential_path(&app)?) {
        Ok(v) => Ok(v),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(String::new()),
        Err(e) => Err(e.to_string()),
    }
}

#[cfg(target_os = "android")]
#[tauri::command]
pub fn clear_credential(app: tauri::AppHandle) -> Result<(), String> {
    match std::fs::remove_file(credential_path(&app)?) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e.to_string()),
    }
}
