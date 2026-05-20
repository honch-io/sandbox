// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Honch Dev

#include <stdbool.h>
#include <stdio.h>
#include <time.h>

#include "app.h"
#include "esp_log.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "esp_sntp.h"
#include "esp_system.h"
#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
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

#ifndef HONCH_SANDBOX_WIFI_SSID
#define HONCH_SANDBOX_WIFI_SSID ""
#endif

#ifndef HONCH_SANDBOX_WIFI_PASSWORD
#define HONCH_SANDBOX_WIFI_PASSWORD ""
#endif

static const char *TAG = "honch_sandbox";

extern volatile bool g_honch_connected;

#ifdef HONCH_SANDBOX_USE_WIFI
#define WIFI_CONNECTED_BIT BIT0
#define WIFI_FAIL_BIT      BIT1
#define WIFI_MAX_RETRY     5

static EventGroupHandle_t s_wifi_event_group;
static int s_retry_num;

static void wifi_event_handler(void *arg, esp_event_base_t event_base, int32_t event_id, void *event_data)
{
    (void)arg;
    if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
    } else if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_DISCONNECTED) {
        if (s_retry_num < WIFI_MAX_RETRY) {
            esp_wifi_connect();
            s_retry_num++;
            ESP_LOGI(TAG, "retrying Wi-Fi connection (%d/%d)", s_retry_num, WIFI_MAX_RETRY);
        } else {
            xEventGroupSetBits(s_wifi_event_group, WIFI_FAIL_BIT);
        }
    } else if (event_base == IP_EVENT && event_id == IP_EVENT_STA_GOT_IP) {
        ip_event_got_ip_t *event = (ip_event_got_ip_t *)event_data;
        ESP_LOGI(TAG, "got IP: " IPSTR, IP2STR(&event->ip_info.ip));
        s_retry_num = 0;
        xEventGroupSetBits(s_wifi_event_group, WIFI_CONNECTED_BIT);
    }
}

static void sandbox_wifi_start(void)
{
    s_wifi_event_group = xEventGroupCreate();
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());
    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    esp_event_handler_instance_t any_id;
    esp_event_handler_instance_t got_ip;
    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        WIFI_EVENT, ESP_EVENT_ANY_ID, &wifi_event_handler, NULL, &any_id));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        IP_EVENT, IP_EVENT_STA_GOT_IP, &wifi_event_handler, NULL, &got_ip));

    wifi_config_t wifi_config = {
        .sta = {
            .ssid = HONCH_SANDBOX_WIFI_SSID,
            .password = HONCH_SANDBOX_WIFI_PASSWORD,
            .threshold.authmode = WIFI_AUTH_WPA2_PSK,
        },
    };

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_config));
    ESP_ERROR_CHECK(esp_wifi_start());

    EventBits_t bits = xEventGroupWaitBits(
        s_wifi_event_group,
        WIFI_CONNECTED_BIT | WIFI_FAIL_BIT,
        pdFALSE,
        pdFALSE,
        portMAX_DELAY);
    if (bits & WIFI_CONNECTED_BIT) {
        ESP_LOGI(TAG, "connected to Wi-Fi");
    } else {
        ESP_LOGE(TAG, "failed to connect to Wi-Fi");
    }
}

static void sandbox_sync_time(void)
{
    ESP_LOGI(TAG, "synchronizing time with SNTP");
    esp_sntp_setoperatingmode(SNTP_OPMODE_POLL);
    esp_sntp_setservername(0, "pool.ntp.org");
    esp_sntp_init();

    time_t now = 0;
    struct tm timeinfo = {0};
    for (int retry = 0; retry < 15; retry++) {
        time(&now);
        localtime_r(&now, &timeinfo);
        if (timeinfo.tm_year >= (2020 - 1900)) {
            ESP_LOGI(TAG, "time synchronized");
            return;
        }
        vTaskDelay(pdMS_TO_TICKS(1000));
    }

    ESP_LOGW(TAG, "time sync timed out; events may use boot-relative timestamps");
}
#endif

void app_main(void)
{
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

#ifdef HONCH_SANDBOX_USE_WIFI
    sandbox_wifi_start();
    sandbox_sync_time();
#else
    sandbox_network_start();
#endif

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

    honch_err_t smoke_status = sandbox_app_track("sdk.esp32.boot", "{\"source\":\"canonical-esp-idf\"}");
    if (smoke_status == HONCH_OK) {
        smoke_status = sandbox_app_flush();
    }
    if (smoke_status != HONCH_OK) {
        ESP_LOGW(TAG, "boot smoke event failed: %d", smoke_status);
    }

    printf("{\"ready\":true,\"adapter\":\"esp-idf\",\"endpoint\":\"%s\"}\n", HONCH_SANDBOX_HOST);
    fflush(stdout);

    sandbox_control_start();
}
