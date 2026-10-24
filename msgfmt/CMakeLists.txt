cmake_minimum_required(VERSION 3.20)
project(msgfmt VERSION 1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(NOT CMAKE_BUILD_TYPE AND NOT CMAKE_CONFIGURATION_TYPES)
  set(CMAKE_BUILD_TYPE Release CACHE STRING "Тип сборки" FORCE)
endif()

option(MSGFMT_WERROR "Считать предупреждения компилятора ошибками" OFF)
option(MSGFMT_SANITIZE "Собрать с -fsanitize=address,undefined" OFF)

# Общие флаги для всех целей проекта.
add_library(msgfmt_options INTERFACE)
if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
  target_compile_options(msgfmt_options INTERFACE -Wall -Wextra -Wpedantic)
  if(MSGFMT_WERROR)
    target_compile_options(msgfmt_options INTERFACE -Werror)
  endif()
  if(MSGFMT_SANITIZE)
    target_compile_options(msgfmt_options INTERFACE
      -fsanitize=address,undefined -fno-sanitize-recover=all -fno-omit-frame-pointer)
    target_link_options(msgfmt_options INTERFACE -fsanitize=address,undefined)
  endif()
endif()

add_library(msgfmt_core STATIC
  src/chars.cpp
  src/escape.cpp
  src/json.cpp
  src/parser.cpp
  src/utf.cpp
)
target_include_directories(msgfmt_core PUBLIC include)
target_link_libraries(msgfmt_core PUBLIC msgfmt_options)

add_executable(msgfmt src/main.cpp)
target_link_libraries(msgfmt PRIVATE msgfmt_core)
