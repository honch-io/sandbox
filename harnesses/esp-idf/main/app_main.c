// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Honch Dev

#include <stdbool.h>
#include <stdio.h>

#include "app.h"
#include "esp_log.h"
#include "esp_system.h"
#include "honch.h"
#include "nvs_flash.h"
#include "sandbox_control.h"
#include "sandbox_network.h"

#ifndef HONCH_SANDBOX_HOST
#define HONCH_SANDBOX_HOST "http://10.0.2.2:18080"
#endif

#ifndef HONCH_SANDBOX_API_KEY
#define HONCH_SANDBOX_API_KEY "honch_e2e_test_key"
#endif

static const char *TAG = "honch_sandbox";

extern volatile bool g_honch_connected;

void app_main(void)
{
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    sandbox_network_start();

    sandbox_app_config_t config = {
        .host = HONCH_SANDBOX_HOST,
        .api_key = HONCH_SANDBOX_API_KEY,
    };
    honch_err_t err = sandbox_app_init(&config);
    if (err != HONCH_OK) {
        ESP_LOGE(TAG, "sandbox_app_init failed: %d", err);
        return;
    }
    g_honch_connected = true;

    printf("{\"ready\":true,\"adapter\":\"esp-idf\",\"endpoint\":\"%s\"}\n", HONCH_SANDBOX_HOST);
    fflush(stdout);

    sandbox_control_start();
}
