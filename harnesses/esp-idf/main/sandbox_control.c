#include "sandbox_control.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "app.h"
#include "cJSON.h"
#include "driver/uart.h"
#include "driver/uart_vfs.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "honch_control";

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
            sandbox_app_set_battery(level->valueint);
            printf("{\"ok\":true,\"battery\":%d}\n", level->valueint);
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
        honch_err_t status = sandbox_app_track(cJSON_IsString(event) ? event->valuestring : "sandbox.event", properties);
        if (properties) {
            free(properties);
        }
        print_status("track", status);
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "flush") == 0) {
        print_status("flush", sandbox_app_flush());
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "reset") == 0) {
        print_status("reset", sandbox_app_reset());
        cJSON_Delete(root);
        return;
    }

    if (strcmp(action->valuestring, "panic") == 0) {
        printf("{\"ok\":true,\"panic\":true}\n");
        fflush(stdout);
        cJSON_Delete(root);
        abort();
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

void sandbox_control_start(void)
{
    setvbuf(stdin, NULL, _IONBF, 0);
    uart_vfs_dev_port_set_rx_line_endings(CONFIG_ESP_CONSOLE_UART_NUM, ESP_LINE_ENDINGS_LF);
    uart_vfs_dev_port_set_tx_line_endings(CONFIG_ESP_CONSOLE_UART_NUM, ESP_LINE_ENDINGS_LF);

    esp_err_t err = uart_driver_install(CONFIG_ESP_CONSOLE_UART_NUM, 4096, 0, 0, NULL, 0);
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE) {
        ESP_LOGW(TAG, "UART control driver install failed: %s", esp_err_to_name(err));
    }
    uart_vfs_dev_use_driver(CONFIG_ESP_CONSOLE_UART_NUM);
    // 8192: the coredump upload path (esp_core_dump_image_get + esp_partition_read
    // + the HTTP post_chunk) needs more stack than the 4096 a plain event flush
    // uses; at 4096 the control task stack-overflows mid coredump upload.
    xTaskCreate(control_task, "honch_control", 8192, NULL, 5, NULL);
}
