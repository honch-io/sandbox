#ifndef HONCH_SANDBOX_JSON_H
#define HONCH_SANDBOX_JSON_H

#include <stddef.h>

int sandbox_json_string(const char *line, const char *field, char *out, size_t out_size);
int sandbox_json_int(const char *line, const char *field, int *out);
int sandbox_json_object(const char *line, const char *field, char *out, size_t out_size);

#endif
