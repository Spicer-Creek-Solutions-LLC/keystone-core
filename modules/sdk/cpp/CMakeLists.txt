cmake_minimum_required(VERSION 3.20)
project(kscore-module-sdk-cpp VERSION 0.1.0)

# Header-only library
add_library(kscore-module-sdk INTERFACE)

target_include_directories(kscore-module-sdk INTERFACE
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)

# C++17 required
target_compile_features(kscore-module-sdk INTERFACE cxx_std_17)

# WASM build configuration
if(CMAKE_SYSTEM_NAME STREQUAL "WASI" OR EMSCRIPTEN)
    message(STATUS "Building for WebAssembly")

    # Size optimization for WASM
    target_compile_options(kscore-module-sdk INTERFACE
        -Os                        # Optimize for size
        -flto                      # Link-time optimization
        -fno-exceptions            # Disable exceptions (smaller binary)
        -fno-rtti                  # Disable RTTI (smaller binary)
    )

    target_link_options(kscore-module-sdk INTERFACE
        -Wl,--strip-all           # Strip all symbols
        -Wl,--gc-sections         # Garbage collect unused sections
    )
endif()

# Example hello-world
add_subdirectory(examples/hello-world)

# Installation
install(DIRECTORY include/kscore DESTINATION include)
