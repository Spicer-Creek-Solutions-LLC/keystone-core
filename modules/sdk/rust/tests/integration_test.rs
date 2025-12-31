//! Integration tests for Keystone Core Rust SDK
//!
//! Note: These tests run in non-WASM mode, so host functions
//! are stubbed and will return errors. For real integration testing,
//! use the Keystone Core test framework with a WASM runtime.

use kscore_module_sdk::{Error, Capability, ModuleContext, ModuleResult};
use kscore_module_sdk::host::*;

#[test]
fn test_capability_as_str() {
    assert_eq!(Capability::FsRead.as_str(), "fs.read");
    assert_eq!(Capability::FsWrite.as_str(), "fs.write");
    assert_eq!(Capability::HttpGet.as_str(), "http.get");
    assert_eq!(Capability::HttpPost.as_str(), "http.post");
    assert_eq!(Capability::Exec.as_str(), "exec");
    assert_eq!(Capability::SecretsRead.as_str(), "secrets.read");
    assert_eq!(Capability::SecretsWrite.as_str(), "secrets.write");
    assert_eq!(Capability::Log.as_str(), "log");
    assert_eq!(Capability::Time.as_str(), "time");
    assert_eq!(Capability::Kv.as_str(), "kv");
}

#[test]
fn test_module_result_ok() {
    let result: ModuleResult<String> = ModuleResult::ok("test".to_string());
    assert!(result.success);
    assert_eq!(result.data, Some("test".to_string()));
    assert_eq!(result.error, None);
}

#[test]
fn test_module_result_err() {
    let result: ModuleResult<String> = ModuleResult::err("failed");
    assert!(!result.success);
    assert_eq!(result.data, None);
    assert_eq!(result.error, Some("failed".to_string()));
}

#[test]
fn test_error_types() {
    let err = Error::capability_denied("fs.read");
    assert!(matches!(err, Error::CapabilityDenied(_)));
    assert_eq!(err.to_string(), "Capability denied: Capability 'fs.read' not granted");

    let err = Error::filesystem("not found");
    assert!(matches!(err, Error::FileSystem(_)));

    let err = Error::http("404");
    assert!(matches!(err, Error::Http(_)));

    let err = Error::exec("command failed");
    assert!(matches!(err, Error::Exec(_)));

    let err = Error::other("generic error");
    assert!(matches!(err, Error::Other(_)));
}

#[test]
fn test_error_from_serde() {
    let json_err = serde_json::from_str::<serde_json::Value>("invalid json");
    assert!(json_err.is_err());

    let err: Error = json_err.unwrap_err().into();
    assert!(matches!(err, Error::Serialization(_)));
}

#[test]
fn test_log_level_as_str() {
    use kscore_module_sdk::types::LogLevel;

    assert_eq!(LogLevel::Debug.as_str(), "debug");
    assert_eq!(LogLevel::Info.as_str(), "info");
    assert_eq!(LogLevel::Warn.as_str(), "warn");
    assert_eq!(LogLevel::Error.as_str(), "error");
}

// Host function tests - these will fail in non-WASM mode
// but validate that the API compiles and has correct signatures

#[test]
fn test_fs_read_stubbed() {
    let result = fs::read_file("/test/path");
    assert!(result.is_err());
}

#[test]
fn test_fs_write_stubbed() {
    let result = fs::write_file("/test/path", b"data");
    assert!(result.is_err());
}

#[test]
fn test_fs_read_string_stubbed() {
    let result = fs::read_string("/test/path");
    assert!(result.is_err());
}

#[test]
fn test_fs_write_string_stubbed() {
    let result = fs::write_string("/test/path", "data");
    assert!(result.is_err());
}

#[test]
fn test_http_get_stubbed() {
    let result = http::get("https://example.com");
    assert!(result.is_err());
}

#[test]
fn test_http_post_stubbed() {
    let result = http::post("https://example.com", b"data");
    assert!(result.is_err());
}

#[test]
fn test_exec_run_stubbed() {
    let result = exec::run("ls", &["-la".to_string()]);
    assert!(result.is_err());
}

#[test]
fn test_exec_run_with_input_stubbed() {
    let result = exec::run_with_input("grep", &["test".to_string()], "data");
    assert!(result.is_err());
}

#[test]
fn test_log_functions() {
    // Log functions don't return values, just ensure they compile
    log::debug("debug message");
    log::info("info message");
    log::warn("warn message");
    log::error("error message");
}

#[test]
fn test_kv_get_stubbed() {
    let result = kv::get("test-key");
    assert!(result.is_err());
}

#[test]
fn test_kv_set_stubbed() {
    let result = kv::set("test-key", "test-value");
    assert!(result.is_err());
}

#[test]
fn test_system_cpu_info_stubbed() {
    let result = system::cpu_info();
    assert!(result.is_err());
}

#[test]
fn test_crypto_sha256_stubbed() {
    let result = crypto::sha256(b"data");
    assert!(result.is_err());
}

#[test]
fn test_crypto_sha256_string_stubbed() {
    let result = crypto::sha256_string("data");
    assert!(result.is_err());
}

#[test]
fn test_module_context_serialization() {
    let ctx = ModuleContext {
        module_name: "test/module".to_string(),
        correlation_id: "test-123".to_string(),
        metadata: serde_json::json!({"key": "value"}),
    };

    let json = serde_json::to_string(&ctx).unwrap();
    let deserialized: ModuleContext = serde_json::from_str(&json).unwrap();

    assert_eq!(ctx.module_name, deserialized.module_name);
    assert_eq!(ctx.correlation_id, deserialized.correlation_id);
}

#[test]
fn test_exec_result_serialization() {
    use kscore_module_sdk::types::ExecResult;

    let result = ExecResult {
        exit_code: 0,
        stdout: "output".to_string(),
        stderr: "".to_string(),
    };

    let json = serde_json::to_string(&result).unwrap();
    let deserialized: ExecResult = serde_json::from_str(&json).unwrap();

    assert_eq!(result.exit_code, deserialized.exit_code);
    assert_eq!(result.stdout, deserialized.stdout);
}

#[test]
fn test_http_response_serialization() {
    use kscore_module_sdk::types::HttpResponse;

    let response = HttpResponse {
        status_code: 200,
        headers: vec![("Content-Type".to_string(), "application/json".to_string())],
        body: b"response body".to_vec(),
    };

    let json = serde_json::to_string(&response).unwrap();
    let deserialized: HttpResponse = serde_json::from_str(&json).unwrap();

    assert_eq!(response.status_code, deserialized.status_code);
    assert_eq!(response.headers, deserialized.headers);
    assert_eq!(response.body, deserialized.body);
}
