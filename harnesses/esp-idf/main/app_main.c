// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Honch Dev

#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "cJSON.h"
#include "driver/gpio.h"
#include "driver/uart.h"
#include "driver/uart_vfs.h"
#include "esp_eth.h"
#include "esp_eth_driver.h"
#include "esp_eth_mac.h"
#include "esp_eth_netif_glue.h"
#include "esp_eth_phy.h"
#include "esp_eth_phy_dp83848.h"
#include "esp_event.h"
#include "esp_log.h"
#include "esp_netif.h"
#include "esp_system.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "freertos/task.h"
#include "honch.h"
#include "nvs_flash.h"

#ifndef HONCH_SANDBOX_HOST
#define HONCH_SANDBOX_HOST "http://10.0.2.2:18080"
#endif

#ifndef HONCH_SANDBOX_API_KEY
#define HONCH_SANDBOX_API_KEY "honch_e2e_test_key"
#endif

static const char *TAG = "honch_sandbox";
static const int ETH_CONNECTED_BIT = BIT0;
static int s_battery_level = 100;
static uint8_t s_event_buffer[16384];
static EventGroupHandle_t s_network_events;
static esp_eth_handle_t s_eth_handle;
static esp_eth_netif_glue_handle_t s_eth_glue;

/*
 * The current ESP-IDF SDK marks connectivity from Wi-Fi lifecycle events only.
 * QEMU uses OpenETH, so the sandbox harness mirrors Ethernet state into the SDK
 * connectivity flag until the SDK exposes a transport-agnostic connectivity API.
 */
extern volatile bool g_honch_connected;

static int sandbox_battery_level(void)
{
    return s_battery_level;
}

static void eth_event_handler(void *arg, esp_event_base_t event_base, int32_t event_id, void *event_data)
{
    (void)arg;
    (void)event_base;
    (void)event_data;

    if (event_id == ETHERNET_EVENT_DISCONNECTED || event_id == ETHERNET_EVENT_STOP) {
        g_honch_connected = false;
    }
}

static void ip_event_handler(void *arg, esp_event_base_t event_base, int32_t event_id, void *event_data)
{
    (void)arg;
    (void)event_base;

    if (event_id != IP_EVENT_ETH_GOT_IP) {
        return;
    }

    ip_event_got_ip_t *event = (ip_event_got_ip_t *)event_data;
    ESP_LOGI(TAG, "QEMU Ethernet IP: " IPSTR, IP2STR(&event->ip_info.ip));
    g_honch_connected = true;
    xEventGroupSetBits(s_network_events, ETH_CONNECTED_BIT);
}

static void start_qemu_ethernet(void)
{
    s_network_events = xEventGroupCreate();
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    esp_netif_config_t netif_config = ESP_NETIF_DEFAULT_ETH();
    esp_netif_t *netif = esp_netif_new(&netif_config);
    ESP_ERROR_CHECK(netif == NULL ? ESP_FAIL : ESP_OK);

    eth_mac_config_t mac_config = ETH_MAC_DEFAULT_CONFIG();
    eth_phy_config_t phy_config = ETH_PHY_DEFAULT_CONFIG();
    phy_config.autonego_timeout_ms = 100;
    esp_eth_mac_t *mac = esp_eth_mac_new_openeth(&mac_config);
    esp_eth_phy_t *phy = esp_eth_phy_new_dp83848(&phy_config);
    esp_eth_config_t eth_config = ETH_DEFAULT_CONFIG(mac, phy);
    ESP_ERROR_CHECK(esp_eth_driver_install(&eth_config, &s_eth_handle));

    uint8_t mac_addr[] = {0x02, 0x00, 0x00, 0x12, 0x34, 0x56};
    ESP_ERROR_CHECK(esp_eth_ioctl(s_eth_handle, ETH_CMD_S_MAC_ADDR, mac_addr));

    s_eth_glue = esp_eth_new_netif_glue(s_eth_handle);
    ESP_ERROR_CHECK(esp_netif_attach(netif, s_eth_glue));
    ESP_ERROR_CHECK(esp_event_handler_register(ETH_EVENT, ESP_EVENT_ANY_ID, eth_event_handler, NULL));
    ESP_ERROR_CHECK(esp_event_handler_register(IP_EVENT, IP_EVENT_ETH_GOT_IP, ip_event_handler, NULL));
    ESP_ERROR_CHECK(esp_eth_start(s_eth_handle));

    xEventGroupWaitBits(s_network_events, ETH_CONNECTED_BIT, pdFALSE, pdTRUE, portMAX_DELAY);
}

static void print_status(const char *name, honch_err_t status)
{
    printf("{\"ok\":%s,\"%s_status\":%d}\n", status == HONCH_OK ? "true" : "false", name, status);
    fflush(stdout);
}

static char *json_properties(cJSON *root)
{
    cJSON *properties = cJSON_GetObjectItem(root, "properties");
    if (!cJSON_IsObject(properties)) {
        char *empty = malloc(3);
        if (empty) {
            strcpy(empty, "{}");
        }
        return empty;
    }
    return cJSON_PrintUnformatted(properties);
}

static void handle_control_line(const char *line)
{
    cJSON *root = cJSON_Parse(line);
    if (!root) {
        printf("{\"ok\":false,\"error\":\"invalid_json\"}\n");
        fflush(stdout);
        return;
    }

    cJSON *action = cJSON_GetObjectItem(root, "action");
    if (!cJSON_IsString(action)) {
        printf("{\"ok\":false,\"error\":\"missing_action\"}\n");
        fflush(stdout);
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "battery") == 0) {
        cJSON *level = cJSON_GetObjectItem(root, "level");
        if (cJSON_IsNumber(level) && level->valueint >= 0 && level->valueint <= 100) {
            s_battery_level = level->valueint;
            printf("{\"ok\":true,\"battery\":%d}\n", s_battery_level);
        } else {
            printf("{\"ok\":false,\"error\":\"invalid_battery\"}\n");
        }
        fflush(stdout);
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "track") == 0) {
        cJSON *event = cJSON_GetObjectItem(root, "event");
        char *properties = json_properties(root);
        honch_err_t status = honch_track(cJSON_IsString(event) ? event->valuestring : "sandbox.event", properties);
        if (properties) {
            free(properties);
        }
        print_status("track", status);
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "flush") == 0) {
        print_status("flush", honch_flush());
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "reset") == 0) {
        print_status("reset", honch_reset());
        cJSON_Delete(root);
        return;
    }

    printf("{\"ok\":false,\"error\":\"unknown_action\"}\n");
    fflush(stdout);
    cJSON_Delete(root);
}

static void control_task(void *arg)
{
    (void)arg;
    char line[2048];
    while (fgets(line, sizeof(line), stdin) != NULL) {
        handle_control_line(line);
    }
    vTaskDelete(NULL);
}

static void start_uart_control(void)
{
    setvbuf(stdin, NULL, _IONBF, 0);
    uart_vfs_dev_port_set_rx_line_endings(CONFIG_ESP_CONSOLE_UART_NUM, ESP_LINE_ENDINGS_LF);
    uart_vfs_dev_port_set_tx_line_endings(CONFIG_ESP_CONSOLE_UART_NUM, ESP_LINE_ENDINGS_LF);

    esp_err_t err = uart_driver_install(CONFIG_ESP_CONSOLE_UART_NUM, 4096, 0, 0, NULL, 0);
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE) {
        ESP_LOGW(TAG, "UART control driver install failed: %s", esp_err_to_name(err));
    }
    uart_vfs_dev_use_driver(CONFIG_ESP_CONSOLE_UART_NUM);
}

void app_main(void)
{
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    start_qemu_ethernet();

    honch_config_t config = {
        .api_key = HONCH_SANDBOX_API_KEY,
        .host = HONCH_SANDBOX_HOST,
        .device_model = "honch-sandbox-esp32-qemu",
        .firmware_version = "sandbox-v2",
        .environment = "sandbox",
        .event_buffer = s_event_buffer,
        .event_buffer_size = sizeof(s_event_buffer),
        .flush_interval_seconds = 3600,
        .flush_event_threshold = 100,
        .battery_callback = sandbox_battery_level,
        .battery_low_threshold = 15,
    };

    honch_err_t err = honch_init(&config);
    if (err != HONCH_OK) {
        ESP_LOGE(TAG, "honch_init failed: %d", err);
        return;
    }
    g_honch_connected = true;

    (void)honch_session_start("sandbox-esp-idf");
    printf("{\"ready\":true,\"adapter\":\"esp-idf\",\"endpoint\":\"%s\"}\n", HONCH_SANDBOX_HOST);
    fflush(stdout);

    start_uart_control();
    xTaskCreate(control_task, "honch_control", 4096, NULL, 5, NULL);
}
