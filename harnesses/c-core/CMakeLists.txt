cmake_minimum_required(VERSION 3.20)

project(honch_sandbox_c_core C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)
set(CMAKE_C_EXTENSIONS OFF)

set(HONCH_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(HONCH_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
add_subdirectory(../../../../c-core c-core-sdk)

add_executable(honch_sandbox_c_core
    app.c
    main.c
    sandbox_control.c
)
target_link_libraries(honch_sandbox_c_core PRIVATE honch_c_core)
target_compile_definitions(honch_sandbox_c_core PRIVATE _POSIX_C_SOURCE=200809L _DEFAULT_SOURCE _DARWIN_C_SOURCE)
target_compile_options(honch_sandbox_c_core PRIVATE
    $<$<C_COMPILER_ID:AppleClang,Clang,GNU>:-Wall -Wextra -Wpedantic -Werror>
)
