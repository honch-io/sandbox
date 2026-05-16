#include "sandbox_json.h"

#include <stdio.h>
#include <string.h>

static int expect_string(void)
{
    char value[64];
    if (!sandbox_json_string("{\"action\": \"battery\"}", "action", value, sizeof(value))) {
        fprintf(stderr, "action field was not parsed\n");
        return 1;
    }
    if (strcmp(value, "battery") != 0) {
        fprintf(stderr, "action = %s\n", value);
        return 1;
    }
    return 0;
}

static int expect_signed_int(void)
{
    int value = 0;
    if (!sandbox_json_int("{\"level\": -1}", "level", &value)) {
        fprintf(stderr, "level field was not parsed\n");
        return 1;
    }
    if (value != -1) {
        fprintf(stderr, "level = %d\n", value);
        return 1;
    }
    return 0;
}

static int expect_object(void)
{
    char value[128];
    const char *line = "{\"properties\": {\"zone\":\"porch\",\"nested\":{\"ok\":true}}}";
    if (!sandbox_json_object(line, "properties", value, sizeof(value))) {
        fprintf(stderr, "properties field was not parsed\n");
        return 1;
    }
    if (strcmp(value, "{\"zone\":\"porch\",\"nested\":{\"ok\":true}}") != 0) {
        fprintf(stderr, "properties = %s\n", value);
        return 1;
    }
    return 0;
}

int main(void)
{
    if (expect_string() != 0 || expect_signed_int() != 0 || expect_object() != 0) {
        return 1;
    }
    return 0;
}
