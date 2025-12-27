//! Core types for TitanAnvil modules

use serde::{Deserialize, Serialize};

/// Capability types available to modules
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum Capability {
    /// File system read capability
    FsRead,
    /// File system write capability
    FsWrite,
    /// HTTP GET capability
    HttpGet,
    /// HTTP POST capability
    HttpPost,
    /// Command execution capability
    Exec,
    /// Secrets read capability
    SecretsRead,
    /// Secrets write capability
    SecretsWrite,
    /// Logging capability
    Log,
    /// Time access capability
    Time,
    /// Key-value storage capability
    Kv,
}

impl Capability {
    /// Convert capability to string representation
    pub fn as_str(&self) -> &'static str {
        match self {
            Capability::FsRead => "fs.read",
            Capability::FsWrite => "fs.write",
            Capability::HttpGet => "http.get",
            Capability::HttpPost => "http.post",
            Capability::Exec => "exec",
            Capability::SecretsRead => "secrets.read",
            Capability::SecretsWrite => "secrets.write",
            Capability::Log => "log",
            Capability::Time => "time",
            Capability::Kv => "kv",
        }
    }
}

/// Module execution context
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModuleContext {
    /// Module name
    pub module_name: String,
    /// Correlation ID for request tracking
    pub correlation_id: String,
    /// Metadata
    #[serde(default)]
    pub metadata: serde_json::Value,
}

/// Module execution result
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModuleResult<T> {
    /// Success status
    pub success: bool,
    /// Result data
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<T>,
    /// Error message if failed
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

impl<T> ModuleResult<T> {
    /// Create a successful result
    pub fn ok(data: T) -> Self {
        Self {
            success: true,
            data: Some(data),
            error: None,
        }
    }

    /// Create a failed result
    pub fn err(error: impl Into<String>) -> Self {
        Self {
            success: false,
            data: None,
            error: Some(error.into()),
        }
    }
}

/// File metadata
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileInfo {
    pub path: String,
    pub size: u64,
    pub is_dir: bool,
    pub modified: u64,
}

/// HTTP request
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpRequest {
    pub url: String,
    #[serde(default)]
    pub headers: Vec<(String, String)>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub body: Option<Vec<u8>>,
}

/// HTTP response
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpResponse {
    pub status_code: u16,
    #[serde(default)]
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

/// Command execution request
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExecRequest {
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stdin: Option<String>,
}

/// Command execution result
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExecResult {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

/// Log level
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
}

impl LogLevel {
    pub fn as_str(&self) -> &'static str {
        match self {
            LogLevel::Debug => "debug",
            LogLevel::Info => "info",
            LogLevel::Warn => "warn",
            LogLevel::Error => "error",
        }
    }
}
