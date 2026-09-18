use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;

#[tauri::command]
fn provision_local_server() -> Result<String, String> {
    let dir = data_dir();
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let dest = dir.join(server_bin_name());
    if !dest.exists() {
        if let Some(src) = find_server_binary() {
            std::fs::copy(&src, &dest).map_err(|e| e.to_string())?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let mut p = std::fs::metadata(&dest).map_err(|e| e.to_string())?.permissions();
                p.set_mode(0o755);
                std::fs::set_permissions(&dest, p).map_err(|e| e.to_string())?;
            }
        } else {
            return Err("matching wayshard-server binary not found; install it beside the app".into());
        }
    }
    // Existing daemon is left running. Window close does not stop it.
    if health_ok() {
        return Ok("http://127.0.0.1:7420".into());
    }
    let mut cmd = Command::new(&dest);
    cmd.arg("--listen")
        .arg("127.0.0.1:7420")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NEW_PROCESS_GROUP: u32 = 0x00000200;
        const DETACHED_PROCESS: u32 = 0x00000008;
        cmd.creation_flags(CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS);
    }
    cmd.spawn().map_err(|e| e.to_string())?;
    for _ in 0..50 {
        thread::sleep(Duration::from_millis(100));
        if health_ok() {
            return Ok("http://127.0.0.1:7420".into());
        }
    }
    Err("server started but did not become healthy".into())
}

fn data_dir() -> PathBuf {
    if let Ok(p) = std::env::var("WAYSHARD_DATA") {
        return PathBuf::from(p);
    }
    let home = std::env::var("HOME")
        .or_else(|_| std::env::var("USERPROFILE"))
        .unwrap_or_else(|_| ".".into());
    if cfg!(target_os = "macos") {
        PathBuf::from(home).join("Library/Application Support/Wayshard")
    } else if cfg!(windows) {
        PathBuf::from(std::env::var("APPDATA").unwrap_or(home)).join("Wayshard")
    } else {
        PathBuf::from(home).join(".local/share/wayshard")
    }
}

fn server_bin_name() -> &'static str {
    if cfg!(windows) {
        "wayshard-server.exe"
    } else {
        "wayshard-server"
    }
}

fn find_server_binary() -> Option<PathBuf> {
    let exe = std::env::current_exe().ok()?;
    let dir = exe.parent()?;
    let name = server_bin_name();
    for p in [dir.join(name), dir.join("bin").join(name)] {
        if p.exists() {
            return Some(p);
        }
    }
    None
}

fn health_ok() -> bool {
    std::net::TcpStream::connect_timeout(
        &"127.0.0.1:7420".parse().unwrap(),
        Duration::from_millis(200),
    )
    .is_ok()
}

pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![provision_local_server])
        .run(tauri::generate_context!())
        .expect("error while running Wayshard");
}
