cmake_minimum_required(VERSION 3.10)
project(tracker C)

set(CMAKE_C_STANDARD 11)

# Source files from tracker subdirectory
set(SOURCES
    tracker/global.c
    tracker/bans.c
    tracker/currentgames.c
    tracker/main.c
    tracker/tcp.c
    tracker/udp.c
    tracker/http.c
    tracker/util.c
    tracker/stats.c
    tracker/interesting.c
        tracker/bans.c
)

add_executable(tracker ${SOURCES})

# Platform-specific configuration
if(WIN32)
    # Windows: link Winsock2 library
    target_link_libraries(tracker ws2_32)
    
    # Define Windows target
    target_compile_definitions(tracker PRIVATE _WIN32)
else()
    # Linux: link pthread library
    target_link_libraries(tracker pthread)
    
    # Use GNU extensions for epoll and other Linux features (C11 needed for static_assert)
    target_compile_options(tracker PRIVATE -std=gnu11)
endif()

# Include directories
target_include_directories(tracker PRIVATE ${CMAKE_SOURCE_DIR}/tracker)
