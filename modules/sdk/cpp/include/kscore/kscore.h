// Keystone Core Module SDK for C++
// Main include file

#pragma once

#include "types.h"
#include "error.h"
#include "host.h"

// Version information
#define KSCORE_SDK_VERSION_MAJOR 0
#define KSCORE_SDK_VERSION_MINOR 1
#define KSCORE_SDK_VERSION_PATCH 0

namespace kscore {

inline std::string sdk_version() {
    return std::to_string(KSCORE_SDK_VERSION_MAJOR) + "." +
           std::to_string(KSCORE_SDK_VERSION_MINOR) + "." +
           std::to_string(KSCORE_SDK_VERSION_PATCH);
}

} // namespace kscore
