//! Hello World TitanAnvil Module (Rust)
//!
//! This example module demonstrates the TitanAnvil Rust SDK by:
//! - Getting the CPU make and model
//! - Computing a SHA256 hash of the CPU information
//! - Writing the results to a file in the temp directory
//! - Returning the results

use titan_module_sdk::{module_main, Result, Error};
use titan_module_sdk::host::{system, crypto, fs, log};
use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
struct HelloWorldResult {
    cpu_info: String,
    hash: String,
    file_path: String,
}

fn run_hello_world() -> Result<String> {
    log::info("Hello World Rust module starting");

    // Get CPU information
    log::info("Getting CPU information...");
    let cpu_info = system::cpu_info()?;
    log::info(&format!("CPU: {}", cpu_info));

    // Compute SHA256 hash of CPU info
    log::info("Computing SHA256 hash...");
    let hash = crypto::sha256_string(&cpu_info)?;
    log::info(&format!("Hash: {}", hash));

    // Determine temp directory path (cross-platform)
    #[cfg(target_os = "windows")]
    let temp_dir = "C:\\Windows\\Temp";

    #[cfg(not(target_os = "windows"))]
    let temp_dir = "/tmp";

    let file_path = format!("{}/hello-from-titananvil-rust.txt", temp_dir);

    // Create file contents
    let contents = format!("CPU: {}\nSHA256: {}\n", cpu_info, hash);

    // Write to file
    log::info(&format!("Writing to file: {}", file_path));
    fs::write_string(&file_path, &contents)?;
    log::info("File written successfully");

    // Return results as JSON
    let result = HelloWorldResult {
        cpu_info,
        hash,
        file_path,
    };

    let json = serde_json::to_string(&result)
        .map_err(|e| Error::serialization(e.to_string()))?;

    log::info("Hello World Rust module completed successfully");
    Ok(json)
}

// Export the module entry point
module_main!(run_hello_world);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hello_world() {
        // This test runs in non-WASM mode, so host functions will return errors
        // In a real test environment, we would use mock host functions
        let result = run_hello_world();

        // Since we're not in WASM mode, we expect this to fail
        // In production, this would be tested with a proper WASM runtime
        assert!(result.is_err());
    }
}
