#ifndef HONCH_SANDBOX_ESP_IDF_APP_H
#define HONCH_SANDBOX_ESP_IDF_APP_H

#include "honch.h"

typedef struct sandbox_app_config {
    const char *host;
    const char *api_key;
} sandbox_app_config_t;

honch_err_t sandbox_app_init(const sandbox_app_config_t *config);
void sandbox_app_set_battery(int level);
honch_err_t sandbox_app_track(const char *event, const char *properties_json);
honch_err_t sandbox_app_flush(void);
honch_err_t sandbox_app_reset(void);

#endif
