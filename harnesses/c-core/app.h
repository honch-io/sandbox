#ifndef HONCH_SANDBOX_C_CORE_APP_H
#define HONCH_SANDBOX_C_CORE_APP_H

#include <honch/honch.h>

typedef struct sandbox_app {
    honch_client_t *client;
    int battery_level;
} sandbox_app_t;

typedef struct sandbox_app_config {
    const char *endpoint;
    const char *token;
    const char *queue_directory;
} sandbox_app_config_t;

honch_status_t sandbox_app_init(sandbox_app_t *app, const sandbox_app_config_t *config);
void sandbox_app_shutdown(sandbox_app_t *app);
void sandbox_app_set_battery(sandbox_app_t *app, int level);
honch_status_t sandbox_app_track(sandbox_app_t *app, const char *event, const char *properties_json);
honch_status_t sandbox_app_flush(sandbox_app_t *app);
honch_status_t sandbox_app_reset(sandbox_app_t *app);

#endif
