idf_component_register(
    SRCS
        "app.c"
        "app_main.c"
        "sandbox_control.c"
        "sandbox_network.c"
    INCLUDE_DIRS "."
    REQUIRES esp_eth esp_netif esp_driver_uart espressif__cjson espressif__dp83848 nvs_flash honch
)

target_compile_definitions(${COMPONENT_LIB} PRIVATE
    HONCH_SANDBOX_HOST="${HONCH_SANDBOX_HOST}"
    HONCH_SANDBOX_API_KEY="${HONCH_SANDBOX_API_KEY}"
)
