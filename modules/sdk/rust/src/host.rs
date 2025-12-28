//! Host function bindings for Keystone Core capabilities
//!
//! These functions are imported from the WASM host and provide access
//! to capabilities like file I/O, HTTP, command execution, etc.

use crate::{Error, Result};
use crate::types::*;

// Import host functions (these are provided by the Keystone Core runtime)
#[cfg(target_arch = "wasm32")]
extern "C" {
    fn host_fs_read(path_ptr: *const u8, path_len: usize, out_ptr: *mut u8, out_len: *mut usize) -> i32;
    fn host_fs_write(path_ptr: *const u8, path_len: usize, data_ptr: *const u8, data_len: usize) -> i32;
    fn host_http_get(url_ptr: *const u8, url_len: usize, out_ptr: *mut u8, out_len: *mut usize) -> i32;
    fn host_http_post(url_ptr: *const u8, url_len: usize, body_ptr: *const u8, body_len: usize, out_ptr: *mut u8, out_len: *mut usize) -> i32;
    fn host_exec(cmd_ptr: *const u8, cmd_len: usize, out_ptr: *mut u8, out_len: *mut usize) -> i32;
    fn host_log(level: i32, msg_ptr: *const u8, msg_len: usize);
    fn host_kv_get(key_ptr: *const u8, key_len: usize, out_ptr: *mut u8, out_len: *mut usize) -> i32;
    fn host_kv_set(key_ptr: *const u8, key_len: usize, val_ptr: *const u8, val_len: usize) -> i32;
}

// Non-WASM stubs for testing
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_fs_read(_path_ptr: *const u8, _path_len: usize, _out_ptr: *mut u8, _out_len: *mut usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_fs_write(_path_ptr: *const u8, _path_len: usize, _data_ptr: *const u8, _data_len: usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_http_get(_url_ptr: *const u8, _url_len: usize, _out_ptr: *mut u8, _out_len: *mut usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_http_post(_url_ptr: *const u8, _url_len: usize, _body_ptr: *const u8, _body_len: usize, _out_ptr: *mut u8, _out_len: *mut usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_exec(_cmd_ptr: *const u8, _cmd_len: usize, _out_ptr: *mut u8, _out_len: *mut usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_log(_level: i32, _msg_ptr: *const u8, _msg_len: usize) {}
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_kv_get(_key_ptr: *const u8, _key_len: usize, _out_ptr: *mut u8, _out_len: *mut usize) -> i32 { -1 }
#[cfg(not(target_arch = "wasm32"))]
unsafe fn host_kv_set(_key_ptr: *const u8, _key_len: usize, _val_ptr: *const u8, _val_len: usize) -> i32 { -1 }

/// File system operations
pub mod fs {
    use super::*;

    /// Read a file's contents
    pub fn read_file(path: &str) -> Result<Vec<u8>> {
        let mut out_buf = vec![0u8; 1024 * 1024]; // 1MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_fs_read(
                path.as_ptr(),
                path.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            out_buf.truncate(out_len);
            Ok(out_buf)
        } else {
            Err(Error::filesystem("failed to read file"))
        }
    }

    /// Read a file as a string
    pub fn read_string(path: &str) -> Result<String> {
        let bytes = read_file(path)?;
        String::from_utf8(bytes).map_err(|e| Error::filesystem(format!("invalid UTF-8: {}", e)))
    }

    /// Write data to a file
    pub fn write_file(path: &str, data: &[u8]) -> Result<()> {
        let result = unsafe {
            host_fs_write(path.as_ptr(), path.len(), data.as_ptr(), data.len())
        };

        if result == 0 {
            Ok(())
        } else {
            Err(Error::filesystem("failed to write file"))
        }
    }

    /// Write a string to a file
    pub fn write_string(path: &str, data: &str) -> Result<()> {
        write_file(path, data.as_bytes())
    }
}

/// HTTP operations
pub mod http {
    use super::*;

    /// Perform an HTTP GET request
    pub fn get(url: &str) -> Result<HttpResponse> {
        let mut out_buf = vec![0u8; 10 * 1024 * 1024]; // 10MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_http_get(
                url.as_ptr(),
                url.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            out_buf.truncate(out_len);
            serde_json::from_slice(&out_buf).map_err(|e| Error::http(format!("failed to parse response: {}", e)))
        } else {
            Err(Error::http("GET request failed"))
        }
    }

    /// Perform an HTTP POST request
    pub fn post(url: &str, body: &[u8]) -> Result<HttpResponse> {
        let mut out_buf = vec![0u8; 10 * 1024 * 1024]; // 10MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_http_post(
                url.as_ptr(),
                url.len(),
                body.as_ptr(),
                body.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            out_buf.truncate(out_len);
            serde_json::from_slice(&out_buf).map_err(|e| Error::http(format!("failed to parse response: {}", e)))
        } else {
            Err(Error::http("POST request failed"))
        }
    }
}

/// Command execution
pub mod exec {
    use super::*;

    /// Execute a command
    pub fn run(command: &str, args: &[String]) -> Result<ExecResult> {
        let request = ExecRequest {
            command: command.to_string(),
            args: args.to_vec(),
            stdin: None,
        };

        let request_json = serde_json::to_vec(&request)?;
        let mut out_buf = vec![0u8; 10 * 1024 * 1024]; // 10MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_exec(
                request_json.as_ptr(),
                request_json.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            out_buf.truncate(out_len);
            serde_json::from_slice(&out_buf).map_err(|e| Error::exec(format!("failed to parse result: {}", e)))
        } else {
            Err(Error::exec("command execution failed"))
        }
    }

    /// Execute a command with input
    pub fn run_with_input(command: &str, args: &[String], stdin: &str) -> Result<ExecResult> {
        let request = ExecRequest {
            command: command.to_string(),
            args: args.to_vec(),
            stdin: Some(stdin.to_string()),
        };

        let request_json = serde_json::to_vec(&request)?;
        let mut out_buf = vec![0u8; 10 * 1024 * 1024]; // 10MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_exec(
                request_json.as_ptr(),
                request_json.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            out_buf.truncate(out_len);
            serde_json::from_slice(&out_buf).map_err(|e| Error::exec(format!("failed to parse result: {}", e)))
        } else {
            Err(Error::exec("command execution failed"))
        }
    }
}

/// Logging functions
pub mod log {
    use super::*;

    fn log_message(level: LogLevel, msg: &str) {
        unsafe {
            host_log(level as i32, msg.as_ptr(), msg.len());
        }
    }

    /// Log a debug message
    pub fn debug(msg: &str) {
        log_message(LogLevel::Debug, msg);
    }

    /// Log an info message
    pub fn info(msg: &str) {
        log_message(LogLevel::Info, msg);
    }

    /// Log a warning message
    pub fn warn(msg: &str) {
        log_message(LogLevel::Warn, msg);
    }

    /// Log an error message
    pub fn error(msg: &str) {
        log_message(LogLevel::Error, msg);
    }
}

/// Key-value storage
pub mod kv {
    use super::*;

    /// Get a value from key-value storage
    pub fn get(key: &str) -> Result<Option<String>> {
        let mut out_buf = vec![0u8; 1024 * 1024]; // 1MB buffer
        let mut out_len: usize = out_buf.len();

        let result = unsafe {
            host_kv_get(
                key.as_ptr(),
                key.len(),
                out_buf.as_mut_ptr(),
                &mut out_len as *mut usize,
            )
        };

        if result == 0 {
            if out_len == 0 {
                Ok(None)
            } else {
                out_buf.truncate(out_len);
                String::from_utf8(out_buf)
                    .map(Some)
                    .map_err(|e| Error::other(format!("invalid UTF-8: {}", e)))
            }
        } else {
            Err(Error::other("kv get failed"))
        }
    }

    /// Set a value in key-value storage
    pub fn set(key: &str, value: &str) -> Result<()> {
        let result = unsafe {
            host_kv_set(
                key.as_ptr(),
                key.len(),
                value.as_ptr(),
                value.len(),
            )
        };

        if result == 0 {
            Ok(())
        } else {
            Err(Error::other("kv set failed"))
        }
    }
}

/// System information utilities
pub mod system {
    use super::*;

    /// Get CPU information
    pub fn cpu_info() -> Result<String> {
        // Try multiple sources for CPU info
        #[cfg(target_os = "linux")]
        {
            if let Ok(info) = super::fs::read_string("/proc/cpuinfo") {
                // Parse model name from /proc/cpuinfo
                for line in info.lines() {
                    if line.starts_with("model name") {
                        if let Some(model) = line.split(':').nth(1) {
                            return Ok(model.trim().to_string());
                        }
                    }
                }
            }
        }

        #[cfg(target_os = "macos")]
        {
            // Use sysctl on macOS
            let result = super::exec::run("sysctl", &["-n".to_string(), "machdep.cpu.brand_string".to_string()])?;
            if result.exit_code == 0 {
                return Ok(result.stdout.trim().to_string());
            }
        }

        #[cfg(target_os = "windows")]
        {
            // Use WMIC on Windows
            let result = super::exec::run("wmic", &["cpu".to_string(), "get".to_string(), "name".to_string()])?;
            if result.exit_code == 0 {
                // Skip header line and get actual CPU name
                let lines: Vec<&str> = result.stdout.lines().collect();
                if lines.len() > 1 {
                    return Ok(lines[1].trim().to_string());
                }
            }
        }

        Err(Error::other("failed to get CPU info"))
    }
}

/// Crypto utilities
pub mod crypto {
    use super::*;

    /// Compute SHA256 hash of data
    pub fn sha256(data: &[u8]) -> Result<String> {
        // For now, use exec to call external sha256sum
        // In the future, we could use a pure Rust crypto library
        let temp_file = "/tmp/kscore-hash-input";
        super::fs::write_file(temp_file, data)?;

        #[cfg(not(target_os = "windows"))]
        let result = super::exec::run("sha256sum", &[temp_file.to_string()])?;

        #[cfg(target_os = "windows")]
        let result = super::exec::run("certutil", &["-hashfile".to_string(), temp_file.to_string(), "SHA256".to_string()])?;

        if result.exit_code == 0 {
            // Parse hash from output
            let hash = result.stdout.split_whitespace().next()
                .ok_or_else(|| Error::other("failed to parse hash"))?;
            Ok(hash.to_string())
        } else {
            Err(Error::other("failed to compute hash"))
        }
    }

    /// Compute SHA256 hash of a string
    pub fn sha256_string(s: &str) -> Result<String> {
        sha256(s.as_bytes())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_log_level() {
        assert_eq!(LogLevel::Info.as_str(), "info");
        assert_eq!(LogLevel::Error.as_str(), "error");
    }
}
