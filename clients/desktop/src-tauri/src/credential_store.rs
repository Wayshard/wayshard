//! Platform-neutral encrypted credential envelope.
//!
//! Only ciphertext plus non-secret metadata is written to app-private storage;
//! key material never leaves the platform keystore. Keeping the envelope logic
//! platform-neutral lets it be deterministically tested with a fake cipher,
//! independently of the Android Keystore binding.

use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

/// CredentialCipher encrypts/decrypts a credential with a platform key that is
/// not exportable (Android Keystore AES-GCM). `encrypt` returns an opaque,
/// self-describing token that the envelope stores verbatim.
pub trait CredentialCipher {
    /// Stable identifier recorded in the envelope so a token written by one
    /// backend is never silently decrypted by another.
    fn name(&self) -> &'static str;
    fn encrypt(&self, plaintext: &[u8]) -> Result<String, String>;
    fn decrypt(&self, token: &str) -> Result<Vec<u8>, String>;
}

const ENVELOPE_VERSION: u32 = 1;

#[derive(Serialize, Deserialize)]
struct Envelope {
    v: u32,
    alg: String,
    data: String,
}

/// save_encrypted encrypts `plaintext` and writes the envelope atomically.
pub fn save_encrypted(
    path: &Path,
    cipher: &dyn CredentialCipher,
    plaintext: &str,
) -> Result<(), String> {
    let token = cipher.encrypt(plaintext.as_bytes())?;
    let envelope = Envelope {
        v: ENVELOPE_VERSION,
        alg: cipher.name().to_string(),
        data: token,
    };
    let bytes = serde_json::to_vec(&envelope).map_err(|e| e.to_string())?;
    write_atomic(path, &bytes)
}

/// load_encrypted reads and decrypts the envelope. A missing file is not an
/// error (first run); any corruption, version/backend mismatch, or failed
/// authentication is a hard error (fail closed).
pub fn load_encrypted(
    path: &Path,
    cipher: &dyn CredentialCipher,
) -> Result<Option<String>, String> {
    let bytes = match std::fs::read(path) {
        Ok(bytes) => bytes,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(format!("read credential: {e}")),
    };
    let envelope: Envelope =
        serde_json::from_slice(&bytes).map_err(|_| "credential file is corrupt".to_string())?;
    if envelope.v != ENVELOPE_VERSION {
        return Err(format!("unsupported credential version {}", envelope.v));
    }
    if envelope.alg != cipher.name() {
        return Err("credential was written by a different keystore backend".to_string());
    }
    let plaintext = cipher.decrypt(&envelope.data)?;
    String::from_utf8(plaintext)
        .map(Some)
        .map_err(|_| "credential is not valid utf-8".to_string())
}

/// clear_file removes the stored credential. A missing file is not an error.
pub fn clear_file(path: &Path) -> Result<(), String> {
    match std::fs::remove_file(path) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(format!("remove credential: {e}")),
    }
}

fn write_atomic(path: &Path, bytes: &[u8]) -> Result<(), String> {
    if let Some(dir) = path.parent() {
        std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    let tmp: PathBuf = match path.file_name() {
        Some(name) => {
            let mut n = name.to_os_string();
            n.push(".tmp");
            path.with_file_name(n)
        }
        None => return Err("credential path has no file name".to_string()),
    };
    std::fs::write(&tmp, bytes).map_err(|e| e.to_string())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = std::fs::set_permissions(&tmp, std::fs::Permissions::from_mode(0o600));
    }
    std::fs::rename(&tmp, path).map_err(|e| e.to_string())?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// FakeCipher is a reversible, deterministic transform used to exercise the
    /// envelope without any platform keystore. It XORs the plaintext so the
    /// stored bytes never contain the plaintext.
    struct FakeCipher;

    impl CredentialCipher for FakeCipher {
        fn name(&self) -> &'static str {
            "test-fake"
        }
        fn encrypt(&self, plaintext: &[u8]) -> Result<String, String> {
            use base64::Engine as _;
            let masked: Vec<u8> = plaintext.iter().map(|b| b ^ 0x5a).collect();
            Ok(base64::engine::general_purpose::STANDARD.encode(masked))
        }
        fn decrypt(&self, token: &str) -> Result<Vec<u8>, String> {
            use base64::Engine as _;
            let raw = base64::engine::general_purpose::STANDARD
                .decode(token)
                .map_err(|_| "bad base64".to_string())?;
            Ok(raw.iter().map(|b| b ^ 0x5a).collect())
        }
    }

    /// FailingCipher simulates an authentication failure (GCM tag mismatch).
    struct FailingCipher;
    impl CredentialCipher for FailingCipher {
        fn name(&self) -> &'static str {
            "test-fake"
        }
        fn encrypt(&self, _plaintext: &[u8]) -> Result<String, String> {
            Err("encrypt failed".to_string())
        }
        fn decrypt(&self, _token: &str) -> Result<Vec<u8>, String> {
            Err("authentication failed".to_string())
        }
    }

    fn temp_path(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "wayshard-credstore-{}-{}",
            std::process::id(),
            name
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        dir.join("credential.json")
    }

    #[test]
    fn round_trips_and_writes_no_plaintext() {
        let path = temp_path("roundtrip");
        save_encrypted(&path, &FakeCipher, "super-secret-token").unwrap();
        let raw = std::fs::read(&path).unwrap();
        let text = String::from_utf8_lossy(&raw);
        assert!(
            !text.contains("super-secret-token"),
            "plaintext must not appear at rest: {text}"
        );
        assert!(text.contains("test-fake"));
        assert_eq!(
            load_encrypted(&path, &FakeCipher).unwrap().as_deref(),
            Some("super-secret-token")
        );
    }

    #[test]
    fn missing_entry_is_first_run() {
        let path = temp_path("missing");
        assert_eq!(load_encrypted(&path, &FakeCipher).unwrap(), None);
    }

    #[test]
    fn overwrite_replaces_previous_value() {
        let path = temp_path("overwrite");
        save_encrypted(&path, &FakeCipher, "first").unwrap();
        save_encrypted(&path, &FakeCipher, "second").unwrap();
        assert_eq!(load_encrypted(&path, &FakeCipher).unwrap().as_deref(), Some("second"));
        assert_eq!(std::fs::read_dir(path.parent().unwrap()).unwrap().count(), 1);
    }

    #[test]
    fn clear_removes_and_is_idempotent() {
        let path = temp_path("clear");
        save_encrypted(&path, &FakeCipher, "value").unwrap();
        clear_file(&path).unwrap();
        assert!(!path.exists());
        clear_file(&path).unwrap(); // already missing is fine
        assert_eq!(load_encrypted(&path, &FakeCipher).unwrap(), None);
    }

    #[test]
    fn corruption_fails_closed() {
        let path = temp_path("corrupt");
        std::fs::write(&path, b"{not json").unwrap();
        assert!(load_encrypted(&path, &FakeCipher).is_err());

        // Valid JSON, corrupt token.
        std::fs::write(&path, br#"{"v":1,"alg":"test-fake","data":"%%%"}"#).unwrap();
        assert!(load_encrypted(&path, &FakeCipher).is_err());

        // Authentication failure fails closed.
        std::fs::write(&path, br#"{"v":1,"alg":"test-fake","data":"AAAA"}"#).unwrap();
        assert!(load_encrypted(&path, &FailingCipher).is_err());
    }

    #[test]
    fn version_and_backend_mismatch_fail_closed() {
        let path = temp_path("mismatch");
        std::fs::write(&path, br#"{"v":99,"alg":"test-fake","data":"AAAA"}"#).unwrap();
        assert!(load_encrypted(&path, &FakeCipher).is_err());

        std::fs::write(&path, br#"{"v":1,"alg":"other-backend","data":"AAAA"}"#).unwrap();
        assert!(load_encrypted(&path, &FakeCipher).is_err());
    }
}
