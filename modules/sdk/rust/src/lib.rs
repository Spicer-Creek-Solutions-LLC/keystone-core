//! Keystone Core Module SDK for Rust
//!
//! This SDK enables building Keystone Core modules in Rust that compile to WebAssembly.
//! Modules can use host capabilities for file I/O, HTTP, command execution, and more.
//!
//! # Example
//!
//! ```rust,no_run
//! use kscore_module_sdk::{module_main, Result};
//!
//! fn run_module() -> Result<String> {
//!     // Your module logic here
//!     Ok("Hello from Keystone Core!".to_string())
//! }
//!
//! module_main!(run_module);
//! ```

pub mod host;
pub mod types;
pub mod error;

pub use error::{Error, Result};
pub use types::{Capability, ModuleContext, ModuleResult};
pub use host::*;

/// Module entry point macro
///
/// This macro sets up the WASM exports for your module's main function.
///
/// # Example
///
/// ```rust,no_run
/// use kscore_module_sdk::{module_main, Result};
///
/// fn run() -> Result<String> {
///     Ok("Success".to_string())
/// }
///
/// module_main!(run);
/// ```
#[macro_export]
macro_rules! module_main {
    ($func:ident) => {
        #[no_mangle]
        pub extern "C" fn module_main() -> i32 {
            match $func() {
                Ok(_) => 0,
                Err(_) => 1,
            }
        }
    };
}

/// Export a function from your module
///
/// # Example
///
/// ```rust,no_run
/// // This macro is reserved for future use
/// // Currently, modules only export a single entry point via module_main!
/// ```
#[macro_export]
macro_rules! export_fn {
    ($func:ident) => {
        #[no_mangle]
        pub extern "C" fn $func() {
            // Function export logic
        }
    };
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_module_result() {
        let result: Result<String> = Ok("test".to_string());
        assert!(result.is_ok());
    }
}
