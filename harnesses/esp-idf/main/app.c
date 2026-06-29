#include "app.h"

#include <stdint.h>

static int s_battery_level = 100;
static uint8_t s_event_buffer[16384];

static int sandbox_battery_level(void)
{
    return s_battery_level;
}

honch_err_t sandbox_app_init(const sandbox_app_config_t *settings)
{
    honch_config_t config = {
        .api_key = settings->api_key,
        .host = settings->host,
        .device_model = "honch-sandbox-esp32-qemu",
        .firmware_version = "sandbox-v2",
        .environment = "sandbox",
        .event_buffer = s_event_buffer,
        .event_buffer_size = sizeof(s_event_buffer),
        .flush_interval_seconds = 3600,
        .flush_event_threshold = 100,
        // Small spacing so a multi-chunk coredump uploads promptly under the
        // driver's repeated flushes (default 15s would take minutes for ~30 chunks).
        .flush_min_interval_ms = 500,
        .battery_callback = sandbox_battery_level,
        .battery_low_threshold = 15,
        .enable_error_tracking = true,
    };

    honch_err_t err = honch_init(&config);
    if (err != HONCH_OK) {
        return err;
    }
    return honch_session_start("sandbox-esp-idf");
}

void sandbox_app_set_battery(int level)
{
    s_battery_level = level;
}

honch_err_t sandbox_app_track(const char *event, const char *properties_json)
{
    const honch_property_t properties[] = {
        honch_prop("properties_json", honch_str(properties_json ? properties_json : "{}"))
    };
    return honch_track(event, properties, 1u);
}

honch_err_t sandbox_app_flush(void)
{
    return honch_flush();
}

honch_err_t sandbox_app_reset(void)
{
    return honch_reset();
}
