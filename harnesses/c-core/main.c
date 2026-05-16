#include "app.h"
#include "sandbox_control.h"

#include <honch/honch.h>
#include <stdio.h>
#include <stdlib.h>

static const char *env_or_default(const char *name, const char *fallback)
{
    const char *value = getenv(name);
    return value && value[0] ? value : fallback;
}

int main(void)
{
    const char *endpoint = env_or_default("HONCH_SANDBOX_ENDPOINT", "http://127.0.0.1:18080");
    sandbox_app_config_t config = {
        .endpoint = endpoint,
        .token = env_or_default("HONCH_SANDBOX_TOKEN", "honch_e2e_test_key"),
        .queue_directory = env_or_default("HONCH_SANDBOX_QUEUE", ".honch-sandbox/c-core-queue"),
    };

    sandbox_app_t app;
    honch_status_t status = sandbox_app_init(&app, &config);
    if (status != HONCH_OK) {
        fprintf(stderr, "sandbox_app_init failed: %s\n", honch_status_string(status));
        return 1;
    }

    printf("{\"ready\":true,\"adapter\":\"c-core\",\"endpoint\":\"%s\"}\n", endpoint);
    fflush(stdout);

    int result = sandbox_control_run(&app, getenv("HONCH_SANDBOX_CONTROL"));
    sandbox_app_shutdown(&app);
    return result;
}
