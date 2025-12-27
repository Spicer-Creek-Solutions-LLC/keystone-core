// TitanAnvil Module SDK for C++
// Core types and structures

#pragma once

#include <cstdint>
#include <string>
#include <vector>
#include <map>
#include <optional>

namespace titan {

// Capability represents a capability that a module can request
enum class Capability {
    FsRead,
    FsWrite,
    HttpGet,
    HttpPost,
    Exec,
    SecretsRead,
    SecretsWrite,
    Log,
    Time,
    Kv
};

// Convert capability to string
inline std::string capability_to_string(Capability cap) {
    switch (cap) {
        case Capability::FsRead: return "fs.read";
        case Capability::FsWrite: return "fs.write";
        case Capability::HttpGet: return "http.get";
        case Capability::HttpPost: return "http.post";
        case Capability::Exec: return "exec";
        case Capability::SecretsRead: return "secrets.read";
        case Capability::SecretsWrite: return "secrets.write";
        case Capability::Log: return "log";
        case Capability::Time: return "time";
        case Capability::Kv: return "kv";
    }
    return "unknown";
}

// Module execution context
struct ModuleContext {
    std::string module_name;
    std::string correlation_id;
    std::map<std::string, std::string> metadata;
};

// Module execution result
template<typename T>
struct ModuleResult {
    bool success;
    std::optional<T> data;
    std::optional<std::string> error;

    static ModuleResult<T> ok(T value) {
        return {true, std::move(value), std::nullopt};
    }

    static ModuleResult<T> err(std::string error_msg) {
        return {false, std::nullopt, std::move(error_msg)};
    }
};

// File metadata
struct FileInfo {
    std::string path;
    uint64_t size;
    bool is_dir;
    uint64_t modified;
};

// HTTP request
struct HttpRequest {
    std::string url;
    std::map<std::string, std::string> headers;
    std::vector<uint8_t> body;
};

// HTTP response
struct HttpResponse {
    uint16_t status_code;
    std::map<std::string, std::string> headers;
    std::vector<uint8_t> body;
};

// Command execution request
struct ExecRequest {
    std::string command;
    std::vector<std::string> args;
    std::optional<std::string> stdin_data;
};

// Command execution result
struct ExecResult {
    int32_t exit_code;
    std::string stdout_data;
    std::string stderr_data;
};

// Log level
enum class LogLevel : int32_t {
    Debug = 0,
    Info = 1,
    Warn = 2,
    Error = 3
};

// Convert log level to string
inline std::string log_level_to_string(LogLevel level) {
    switch (level) {
        case LogLevel::Debug: return "debug";
        case LogLevel::Info: return "info";
        case LogLevel::Warn: return "warn";
        case LogLevel::Error: return "error";
    }
    return "unknown";
}

} // namespace titan
