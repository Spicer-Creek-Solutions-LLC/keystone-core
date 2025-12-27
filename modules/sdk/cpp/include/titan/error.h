// TitanAnvil Module SDK for C++
// Error types and exceptions

#pragma once

#include <stdexcept>
#include <string>

namespace titan {

// Error type enumeration
enum class ErrorType {
    CapabilityDenied,
    FileSystem,
    Http,
    Exec,
    Serialization,
    Other
};

// Base exception class for TitanAnvil module errors
class Error : public std::runtime_error {
public:
    Error(ErrorType type, const std::string& message)
        : std::runtime_error(format_error(type, message))
        , type_(type)
        , message_(message) {}

    ErrorType type() const { return type_; }
    const std::string& message() const { return message_; }

private:
    ErrorType type_;
    std::string message_;

    static std::string format_error(ErrorType type, const std::string& message) {
        switch (type) {
            case ErrorType::CapabilityDenied:
                return "Capability denied: " + message;
            case ErrorType::FileSystem:
                return "Filesystem error: " + message;
            case ErrorType::Http:
                return "HTTP error: " + message;
            case ErrorType::Exec:
                return "Exec error: " + message;
            case ErrorType::Serialization:
                return "Serialization error: " + message;
            case ErrorType::Other:
                return message;
        }
        return message;
    }
};

// Specific error types

class CapabilityDeniedError : public Error {
public:
    explicit CapabilityDeniedError(const std::string& capability)
        : Error(ErrorType::CapabilityDenied, "Capability '" + capability + "' not granted") {}
};

class FileSystemError : public Error {
public:
    explicit FileSystemError(const std::string& message)
        : Error(ErrorType::FileSystem, message) {}
};

class HttpError : public Error {
public:
    explicit HttpError(const std::string& message)
        : Error(ErrorType::Http, message) {}
};

class ExecError : public Error {
public:
    explicit ExecError(const std::string& message)
        : Error(ErrorType::Exec, message) {}
};

class SerializationError : public Error {
public:
    explicit SerializationError(const std::string& message)
        : Error(ErrorType::Serialization, message) {}
};

} // namespace titan
