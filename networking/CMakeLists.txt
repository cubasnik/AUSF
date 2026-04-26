cmake_minimum_required(VERSION 3.10)
project(AUSF_Networking CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(Threads REQUIRED)

include_directories(${CMAKE_CURRENT_SOURCE_DIR}/include)

set(LIBRARY_SOURCES
    src/pfcp_handler.cpp
)

add_library(ausf_networking ${LIBRARY_SOURCES})
target_link_libraries(ausf_networking
    Threads::Threads
)

add_executable(networking_test src/main.cpp)
target_link_libraries(networking_test ausf_networking)
