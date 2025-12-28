// Keystone Core Module SDK for C++
// Host function bindings for WASM capabilities

#pragma once

#include "types.h"
#include "error.h"
#include <cstring>
#include <sstream>
#include <algorithm>

// Platform detection
#if defined(__wasm__) || defined(__wasm32__) || defined(__EMSCRIPTEN__)
#define KSCORE_WASM 1
#else
#define KSCORE_WASM 0
#endif

// Host function imports (provided by Keystone Core runtime)
#if KSCORE_WASM
extern "C" {
    int32_t host_fs_read(const uint8_t* path_ptr, uint32_t path_len, uint8_t* out_ptr, uint32_t* out_len);
    int32_t host_fs_write(const uint8_t* path_ptr, uint32_t path_len, const uint8_t* data_ptr, uint32_t data_len);
    int32_t host_http_get(const uint8_t* url_ptr, uint32_t url_len, uint8_t* out_ptr, uint32_t* out_len);
    int32_t host_http_post(const uint8_t* url_ptr, uint32_t url_len, const uint8_t* body_ptr, uint32_t body_len, uint8_t* out_ptr, uint32_t* out_len);
    int32_t host_exec(const uint8_t* cmd_ptr, uint32_t cmd_len, uint8_t* out_ptr, uint32_t* out_len);
    void host_log(int32_t level, const uint8_t* msg_ptr, uint32_t msg_len);
    int32_t host_kv_get(const uint8_t* key_ptr, uint32_t key_len, uint8_t* out_ptr, uint32_t* out_len);
    int32_t host_kv_set(const uint8_t* key_ptr, uint32_t key_len, const uint8_t* val_ptr, uint32_t val_len);
}
#endif

namespace kscore {

// Simple JSON parser/builder (minimal implementation to avoid dependencies)
namespace json {
    inline std::string escape(const std::string& s) {
        std::string result;
        for (char c : s) {
            switch (c) {
                case '"': result += "\\\""; break;
                case '\\': result += "\\\\"; break;
                case '\n': result += "\\n"; break;
                case '\r': result += "\\r"; break;
                case '\t': result += "\\t"; break;
                default: result += c;
            }
        }
        return result;
    }

    inline std::string to_json(const HttpResponse& resp) {
        std::ostringstream oss;
        oss << "{\"status_code\":" << resp.status_code << ",\"body\":[";
        for (size_t i = 0; i < resp.body.size(); ++i) {
            if (i > 0) oss << ",";
            oss << static_cast<int>(resp.body[i]);
        }
        oss << "]}";
        return oss.str();
    }

    inline std::string to_json(const ExecRequest& req) {
        std::ostringstream oss;
        oss << "{\"command\":\"" << escape(req.command) << "\",\"args\":[";
        for (size_t i = 0; i < req.args.size(); ++i) {
            if (i > 0) oss << ",";
            oss << "\"" << escape(req.args[i]) << "\"";
        }
        oss << "]";
        if (req.stdin_data) {
            oss << ",\"stdin\":\"" << escape(*req.stdin_data) << "\"";
        }
        oss << "}";
        return oss.str();
    }

    // Minimal JSON parser for exec results
    inline ExecResult parse_exec_result(const std::string& json_str) {
        ExecResult result{0, "", ""};

        // Very simple parser - find exit_code, stdout, stderr fields
        size_t pos = json_str.find("\"exit_code\":");
        if (pos != std::string::npos) {
            result.exit_code = std::stoi(json_str.substr(pos + 12));
        }

        // Find stdout
        pos = json_str.find("\"stdout\":\"");
        if (pos != std::string::npos) {
            size_t start = pos + 10;
            size_t end = json_str.find("\"", start);
            if (end != std::string::npos) {
                result.stdout_data = json_str.substr(start, end - start);
            }
        }

        // Find stderr
        pos = json_str.find("\"stderr\":\"");
        if (pos != std::string::npos) {
            size_t start = pos + 10;
            size_t end = json_str.find("\"", start);
            if (end != std::string::npos) {
                result.stderr_data = json_str.substr(start, end - start);
            }
        }

        return result;
    }
}

// Filesystem operations
namespace fs {
    inline std::vector<uint8_t> read(const std::string& path) {
        #if KSCORE_WASM
        std::vector<uint8_t> output(1024 * 1024); // 1MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_fs_read(
            reinterpret_cast<const uint8_t*>(path.data()),
            path.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            output.resize(out_len);
            return output;
        }
        throw FileSystemError("failed to read file");
        #else
        throw FileSystemError("not running in WASM environment");
        #endif
    }

    inline std::string read_string(const std::string& path) {
        auto data = read(path);
        return std::string(data.begin(), data.end());
    }

    inline void write(const std::string& path, const std::vector<uint8_t>& data) {
        #if KSCORE_WASM
        int32_t result = host_fs_write(
            reinterpret_cast<const uint8_t*>(path.data()),
            path.size(),
            data.data(),
            data.size()
        );

        if (result != 0) {
            throw FileSystemError("failed to write file");
        }
        #else
        throw FileSystemError("not running in WASM environment");
        #endif
    }

    inline void write_string(const std::string& path, const std::string& data) {
        write(path, std::vector<uint8_t>(data.begin(), data.end()));
    }
}

// HTTP operations
namespace http {
    inline HttpResponse get(const std::string& url) {
        #if KSCORE_WASM
        std::vector<uint8_t> output(10 * 1024 * 1024); // 10MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_http_get(
            reinterpret_cast<const uint8_t*>(url.data()),
            url.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            output.resize(out_len);
            std::string json_str(output.begin(), output.end());

            // Simple parsing of JSON response
            HttpResponse response;
            size_t pos = json_str.find("\"status_code\":");
            if (pos != std::string::npos) {
                response.status_code = std::stoi(json_str.substr(pos + 14));
            }

            // Extract body array
            pos = json_str.find("\"body\":[");
            if (pos != std::string::npos) {
                size_t start = pos + 8;
                size_t end = json_str.find("]", start);
                if (end != std::string::npos) {
                    std::string body_str = json_str.substr(start, end - start);
                    // Parse comma-separated numbers
                    std::istringstream iss(body_str);
                    std::string num;
                    while (std::getline(iss, num, ',')) {
                        response.body.push_back(static_cast<uint8_t>(std::stoi(num)));
                    }
                }
            }

            return response;
        }
        throw HttpError("GET request failed");
        #else
        throw HttpError("not running in WASM environment");
        #endif
    }

    inline HttpResponse post(const std::string& url, const std::vector<uint8_t>& body) {
        #if KSCORE_WASM
        std::vector<uint8_t> output(10 * 1024 * 1024); // 10MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_http_post(
            reinterpret_cast<const uint8_t*>(url.data()),
            url.size(),
            body.data(),
            body.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            output.resize(out_len);
            std::string json_str(output.begin(), output.end());

            HttpResponse response;
            size_t pos = json_str.find("\"status_code\":");
            if (pos != std::string::npos) {
                response.status_code = std::stoi(json_str.substr(pos + 14));
            }

            pos = json_str.find("\"body\":[");
            if (pos != std::string::npos) {
                size_t start = pos + 8;
                size_t end = json_str.find("]", start);
                if (end != std::string::npos) {
                    std::string body_str = json_str.substr(start, end - start);
                    std::istringstream iss(body_str);
                    std::string num;
                    while (std::getline(iss, num, ',')) {
                        response.body.push_back(static_cast<uint8_t>(std::stoi(num)));
                    }
                }
            }

            return response;
        }
        throw HttpError("POST request failed");
        #else
        throw HttpError("not running in WASM environment");
        #endif
    }
}

// Command execution
namespace exec {
    inline ExecResult run(const std::string& command, const std::vector<std::string>& args = {}) {
        #if KSCORE_WASM
        ExecRequest req{command, args, std::nullopt};
        std::string request_json = json::to_json(req);

        std::vector<uint8_t> output(10 * 1024 * 1024); // 10MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_exec(
            reinterpret_cast<const uint8_t*>(request_json.data()),
            request_json.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            output.resize(out_len);
            std::string json_str(output.begin(), output.end());
            return json::parse_exec_result(json_str);
        }
        throw ExecError("command execution failed");
        #else
        throw ExecError("not running in WASM environment");
        #endif
    }

    inline ExecResult run_with_input(const std::string& command, const std::string& stdin_data, const std::vector<std::string>& args = {}) {
        #if KSCORE_WASM
        ExecRequest req{command, args, stdin_data};
        std::string request_json = json::to_json(req);

        std::vector<uint8_t> output(10 * 1024 * 1024); // 10MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_exec(
            reinterpret_cast<const uint8_t*>(request_json.data()),
            request_json.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            output.resize(out_len);
            std::string json_str(output.begin(), output.end());
            return json::parse_exec_result(json_str);
        }
        throw ExecError("command execution failed");
        #else
        throw ExecError("not running in WASM environment");
        #endif
    }
}

// Logging
namespace log {
    inline void debug(const std::string& message) {
        #if KSCORE_WASM
        host_log(
            static_cast<int32_t>(LogLevel::Debug),
            reinterpret_cast<const uint8_t*>(message.data()),
            message.size()
        );
        #endif
    }

    inline void info(const std::string& message) {
        #if KSCORE_WASM
        host_log(
            static_cast<int32_t>(LogLevel::Info),
            reinterpret_cast<const uint8_t*>(message.data()),
            message.size()
        );
        #endif
    }

    inline void warn(const std::string& message) {
        #if KSCORE_WASM
        host_log(
            static_cast<int32_t>(LogLevel::Warn),
            reinterpret_cast<const uint8_t*>(message.data()),
            message.size()
        );
        #endif
    }

    inline void error(const std::string& message) {
        #if KSCORE_WASM
        host_log(
            static_cast<int32_t>(LogLevel::Error),
            reinterpret_cast<const uint8_t*>(message.data()),
            message.size()
        );
        #endif
    }
}

// Key-value storage
namespace kv {
    inline std::optional<std::string> get(const std::string& key) {
        #if KSCORE_WASM
        std::vector<uint8_t> output(1024 * 1024); // 1MB buffer
        uint32_t out_len = output.size();

        int32_t result = host_kv_get(
            reinterpret_cast<const uint8_t*>(key.data()),
            key.size(),
            output.data(),
            &out_len
        );

        if (result == 0) {
            if (out_len == 0) {
                return std::nullopt;
            }
            output.resize(out_len);
            return std::string(output.begin(), output.end());
        }
        throw Error(ErrorType::Other, "kv get failed");
        #else
        throw Error(ErrorType::Other, "not running in WASM environment");
        #endif
    }

    inline void set(const std::string& key, const std::string& value) {
        #if KSCORE_WASM
        int32_t result = host_kv_set(
            reinterpret_cast<const uint8_t*>(key.data()),
            key.size(),
            reinterpret_cast<const uint8_t*>(value.data()),
            value.size()
        );

        if (result != 0) {
            throw Error(ErrorType::Other, "kv set failed");
        }
        #else
        throw Error(ErrorType::Other, "not running in WASM environment");
        #endif
    }
}

// System information
namespace system {
    inline std::string get_cpu_info() {
        // Try different methods based on platform
        #if KSCORE_WASM
        #ifdef __linux__
        try {
            std::string cpuinfo = fs::read_string("/proc/cpuinfo");
            size_t pos = cpuinfo.find("model name");
            if (pos != std::string::npos) {
                size_t colon = cpuinfo.find(":", pos);
                size_t newline = cpuinfo.find("\n", colon);
                if (colon != std::string::npos && newline != std::string::npos) {
                    std::string model = cpuinfo.substr(colon + 1, newline - colon - 1);
                    // Trim whitespace
                    model.erase(0, model.find_first_not_of(" \t"));
                    model.erase(model.find_last_not_of(" \t") + 1);
                    return model;
                }
            }
        } catch (...) {}
        #endif

        #ifdef __APPLE__
        try {
            auto result = exec::run("sysctl", {"-n", "machdep.cpu.brand_string"});
            if (result.exit_code == 0) {
                std::string cpu = result.stdout_data;
                cpu.erase(cpu.find_last_not_of(" \t\r\n") + 1);
                return cpu;
            }
        } catch (...) {}
        #endif

        #ifdef _WIN32
        try {
            auto result = exec::run("wmic", {"cpu", "get", "name"});
            if (result.exit_code == 0) {
                // Skip header line
                size_t newline = result.stdout_data.find("\n");
                if (newline != std::string::npos) {
                    std::string cpu = result.stdout_data.substr(newline + 1);
                    cpu.erase(0, cpu.find_first_not_of(" \t\r\n"));
                    cpu.erase(cpu.find_last_not_of(" \t\r\n") + 1);
                    return cpu;
                }
            }
        } catch (...) {}
        #endif

        throw Error(ErrorType::Other, "failed to get CPU info");
        #else
        throw Error(ErrorType::Other, "not running in WASM environment");
        #endif
    }
}

// Cryptography
namespace crypto {
    inline std::string sha256(const std::vector<uint8_t>& data) {
        #if KSCORE_WASM
        // Write to temp file
        std::string temp_file = "/tmp/kscore-hash-input";
        fs::write(temp_file, data);

        #ifndef _WIN32
        auto result = exec::run("sha256sum", {temp_file});
        #else
        auto result = exec::run("certutil", {"-hashfile", temp_file, "SHA256"});
        #endif

        if (result.exit_code == 0) {
            // Parse hash from output (first word)
            size_t space = result.stdout_data.find(" ");
            if (space != std::string::npos) {
                return result.stdout_data.substr(0, space);
            }
            // For single-word output
            std::string hash = result.stdout_data;
            hash.erase(hash.find_last_not_of(" \t\r\n") + 1);
            return hash;
        }

        throw Error(ErrorType::Other, "failed to compute hash");
        #else
        throw Error(ErrorType::Other, "not running in WASM environment");
        #endif
    }

    inline std::string sha256_string(const std::string& s) {
        return sha256(std::vector<uint8_t>(s.begin(), s.end()));
    }
}

} // namespace kscore
