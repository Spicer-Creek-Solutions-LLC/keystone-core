//! Error types for TitanAnvil modules

use std::fmt;

/// Result type for module operations
pub type Result<T> = std::result::Result<T, Error>;

/// Module error types
#[derive(Debug, Clone)]
pub enum Error {
    /// Capability not granted
    CapabilityDenied(String),
    /// File system error
    FileSystem(String),
    /// HTTP error
    Http(String),
    /// Exec error
    Exec(String),
    /// Serialization error
    Serialization(String),
    /// Generic error
    Other(String),
}

impl Error {
    /// Create a capability denied error
    pub fn capability_denied(cap: &str) -> Self {
        Error::CapabilityDenied(format!("Capability '{}' not granted", cap))
    }

    /// Create a filesystem error
    pub fn filesystem(msg: impl Into<String>) -> Self {
        Error::FileSystem(msg.into())
    }

    /// Create an HTTP error
    pub fn http(msg: impl Into<String>) -> Self {
        Error::Http(msg.into())
    }

    /// Create an exec error
    pub fn exec(msg: impl Into<String>) -> Self {
        Error::Exec(msg.into())
    }

    /// Create a serialization error
    pub fn serialization(msg: impl Into<String>) -> Self {
        Error::Serialization(msg.into())
    }

    /// Create a generic error
    pub fn other(msg: impl Into<String>) -> Self {
        Error::Other(msg.into())
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::CapabilityDenied(msg) => write!(f, "Capability denied: {}", msg),
            Error::FileSystem(msg) => write!(f, "Filesystem error: {}", msg),
            Error::Http(msg) => write!(f, "HTTP error: {}", msg),
            Error::Exec(msg) => write!(f, "Exec error: {}", msg),
            Error::Serialization(msg) => write!(f, "Serialization error: {}", msg),
            Error::Other(msg) => write!(f, "{}", msg),
        }
    }
}

impl std::error::Error for Error {}

impl From<serde_json::Error> for Error {
    fn from(err: serde_json::Error) -> Self {
        Error::Serialization(err.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_error_display() {
        let err = Error::capability_denied("fs.read");
        assert_eq!(err.to_string(), "Capability denied: Capability 'fs.read' not granted");
    }

    #[test]
    fn test_error_types() {
        let err = Error::filesystem("not found");
        assert!(matches!(err, Error::FileSystem(_)));

        let err = Error::http("404");
        assert!(matches!(err, Error::Http(_)));
    }
}
