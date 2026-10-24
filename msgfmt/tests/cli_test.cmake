cmake_minimum_required(VERSION 3.20)

# Запуск: cmake -DBIN=... -DARGS=... -DINPUT=... -DEXPECTED=... -DEXPECTED_CODE=... -P cli_test.cmake
foreach(var IN ITEMS BIN INPUT EXPECTED EXPECTED_CODE)
  if(NOT DEFINED ${var})
    message(FATAL_ERROR "cli_test.cmake: не задан ${var}")
  endif()
endforeach()

execute_process(
  COMMAND ${BIN} ${ARGS}
  INPUT_FILE ${INPUT}
  OUTPUT_VARIABLE actual
  ERROR_VARIABLE stderr
  RESULT_VARIABLE code
)
file(READ ${EXPECTED} expected)

if(NOT code STREQUAL EXPECTED_CODE)
  message(FATAL_ERROR "код возврата ${code}, ожидался ${EXPECTED_CODE}\nstderr: ${stderr}")
endif()
if(NOT actual STREQUAL expected)
  message(FATAL_ERROR "stdout не совпал\nполучено:  [${actual}]\nожидалось: [${expected}]")
endif()
