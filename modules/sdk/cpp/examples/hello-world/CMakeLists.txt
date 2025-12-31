cmake_minimum_required(VERSION 3.20)
project(hello-world-cpp)

# Add the Keystone Core SDK
add_subdirectory(../.. kscore-sdk)

# Create the hello-world executable
add_executable(hello-world main.cpp)

target_link_libraries(hello-world PRIVATE kscore-module-sdk)

# WASM-specific settings
if(CMAKE_SYSTEM_NAME STREQUAL "WASI" OR EMSCRIPTEN)
    # Set output to .wasm
    set_target_properties(hello-world PROPERTIES
        SUFFIX ".wasm"
        OUTPUT_NAME "hello-world-cpp"
    )

    # Export main function
    if(EMSCRIPTEN)
        target_link_options(hello-world PRIVATE
            -sEXPORTED_FUNCTIONS=_main
            -sSTANDALONE_WASM=1
        )
    endif()
endif()
