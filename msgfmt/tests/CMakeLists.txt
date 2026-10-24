add_executable(msgfmt_tests
  harness.cpp
  test_chars.cpp
  test_escape.cpp
  test_json.cpp
  test_utf.cpp
)
target_link_libraries(msgfmt_tests PRIVATE msgfmt_core)

foreach(suite IN ITEMS chars escape json utf)
  add_test(NAME unit.${suite} COMMAND msgfmt_tests ${suite})
endforeach()

# CLI-тест: cli/<name>.in подаётся на stdin, stdout сравнивается с cli/<name>.out,
# код возврата — с expected_code. Дополнительные аргументы передаются msgfmt.
function(msgfmt_cli_test name expected_code)
  add_test(NAME cli.${name}
    COMMAND ${CMAKE_COMMAND}
      -DBIN=$<TARGET_FILE:msgfmt>
      "-DARGS=${ARGN}"
      -DINPUT=${CMAKE_CURRENT_SOURCE_DIR}/cli/${name}.in
      -DEXPECTED=${CMAKE_CURRENT_SOURCE_DIR}/cli/${name}.out
      -DEXPECTED_CODE=${expected_code}
      -P ${CMAKE_CURRENT_SOURCE_DIR}/cli_test.cmake)
endfunction()

msgfmt_cli_test(format_empty 0)
msgfmt_cli_test(format_plain 0)
msgfmt_cli_test(format_invalid_utf8 2)
msgfmt_cli_test(escape_example 0 escape)
msgfmt_cli_test(escape_all_service 0 escape)
msgfmt_cli_test(escape_empty 0 escape)
msgfmt_cli_test(escape_invalid_utf8 2 escape)
