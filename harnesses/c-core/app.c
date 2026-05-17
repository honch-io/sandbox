#include "app.h"

#include <stddef.h>

static sandbox_app_t *g_active_app;

static int sandbox_battery_level(void)
{
    if (!g_active_app) {
        return 100;
    }
    return g_active_app->battery_level;
}

honch_status_t sandbox_app_init(sandbox_app_t *app, const sandbox_app_config_t *settings)
{
    if (!app || !settings) {
        return HONCH_ERROR_INVALID_ARGUMENT;
    }

    app->client = NULL;
    app->battery_level = 100;
    g_active_app = app;

    honch_config_t config = {
        .api_key = settings->token,
        .endpoint_url = settings->endpoint,
        .device_model = "honch-sandbox-c-core",
        .firmware_version = "sandbox-v1",
        .environment = "sandbox",
        .queue_directory = settings->queue_directory,
        .batch_size = 5,
        .transport_timeout_ms = 3000,
        .disable_background_flush = 1,
        .battery_callback = sandbox_battery_level,
        .battery_low_threshold = 15,
    };

    honch_status_t status = honch_init(&app->client, &config);
    if (status != HONCH_OK) {
        g_active_app = NULL;
        return status;
    }
    return honch_session_start(app->client, "sandbox-c-core");
}

void sandbox_app_shutdown(sandbox_app_t *app)
{
    if (!app || !app->client) {
        return;
    }
    (void)honch_session_end(app->client);
    (void)honch_shutdown(app->client);
    if (g_active_app == app) {
        g_active_app = NULL;
    }
    app->client = NULL;
}

void sandbox_app_set_battery(sandbox_app_t *app, int level)
{
    if (!app) {
        return;
    }
    app->battery_level = level;
}

honch_status_t sandbox_app_track(sandbox_app_t *app, const char *event, const char *properties_json)
{
    if (!app || !app->client) {
        return HONCH_ERROR_INVALID_ARGUMENT;
    }
    return honch_track(app->client, event, properties_json);
}

honch_status_t sandbox_app_flush(sandbox_app_t *app)
{
    if (!app || !app->client) {
        return HONCH_ERROR_INVALID_ARGUMENT;
    }
    return honch_flush(app->client);
}

honch_status_t sandbox_app_reset(sandbox_app_t *app)
{
    if (!app || !app->client) {
        return HONCH_ERROR_INVALID_ARGUMENT;
    }
    return honch_reset(app->client);
}
