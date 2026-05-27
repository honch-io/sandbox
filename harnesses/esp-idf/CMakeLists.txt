cmake_minimum_required(VERSION 3.16)

set(EXTRA_COMPONENT_DIRS "../../../SDK/ports/esp-idf/honch")
set(HONCH_SANDBOX_HOST "http://10.0.2.2:18080" CACHE STRING "Honch sandbox proxy URL visible from QEMU")
set(HONCH_SANDBOX_API_KEY "honch_e2e_test_key" CACHE STRING "Honch sandbox API key")
set(HONCH_SANDBOX_USE_WIFI OFF CACHE BOOL "Use Wi-Fi instead of QEMU Ethernet")
set(HONCH_SANDBOX_WIFI_SSID "" CACHE STRING "Wi-Fi SSID for hardware runs")
set(HONCH_SANDBOX_WIFI_PASSWORD "" CACHE STRING "Wi-Fi password for hardware runs")

include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(honch_sandbox_esp_idf)
