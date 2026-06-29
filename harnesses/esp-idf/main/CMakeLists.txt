idf_component_register(
    SRCS
        "app.c"
        "app_main.c"
        "sandbox_control.c"
        "sandbox_network.c"
    INCLUDE_DIRS "."
    REQUIRES esp_eth esp_netif esp_wifi esp_driver_gpio esp_driver_uart nvs_flash espressif__cjson espressif__dp83848 honch
)

target_compile_definitions(${COMPONENT_LIB} PRIVATE
    HONCH_SANDBOX_HOST="${HONCH_SANDBOX_HOST}"
    HONCH_SANDBOX_API_KEY="${HONCH_SANDBOX_API_KEY}"
)

if(HONCH_SANDBOX_USE_WIFI)
    target_compile_definitions(${COMPONENT_LIB} PRIVATE
        HONCH_SANDBOX_USE_WIFI=1
        HONCH_SANDBOX_WIFI_SSID="${HONCH_SANDBOX_WIFI_SSID}"
        HONCH_SANDBOX_WIFI_PASSWORD="${HONCH_SANDBOX_WIFI_PASSWORD}"
    )
endif()
