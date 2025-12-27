// TitanAnvil Module SDK for C++
// Main include file

#pragma once

#include "types.h"
#include "error.h"
#include "host.h"

// Version information
#define TITAN_SDK_VERSION_MAJOR 0
#define TITAN_SDK_VERSION_MINOR 1
#define TITAN_SDK_VERSION_PATCH 0

namespace titan {

inline std::string sdk_version() {
    return std::to_string(TITAN_SDK_VERSION_MAJOR) + "." +
           std::to_string(TITAN_SDK_VERSION_MINOR) + "." +
           std::to_string(TITAN_SDK_VERSION_PATCH);
}

} // namespace titan
