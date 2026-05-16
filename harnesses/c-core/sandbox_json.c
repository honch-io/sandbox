#include "sandbox_json.h"

#include <ctype.h>
#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static const char *field_value(const char *line, const char *field)
{
    char pattern[64];
    if (!line || !field || snprintf(pattern, sizeof(pattern), "\"%s\"", field) >= (int)sizeof(pattern)) {
        return NULL;
    }
    const char *cursor = strstr(line, pattern);
    if (!cursor) {
        return NULL;
    }
    cursor = strchr(cursor + strlen(pattern), ':');
    if (!cursor) {
        return NULL;
    }
    cursor++;
    while (*cursor && isspace((unsigned char)*cursor)) {
        cursor++;
    }
    return cursor;
}

int sandbox_json_string(const char *line, const char *field, char *out, size_t out_size)
{
    if (!out || out_size == 0) {
        return 0;
    }
    out[0] = '\0';
    const char *cursor = field_value(line, field);
    if (!cursor || *cursor != '"') {
        return 0;
    }
    cursor++;
    size_t i = 0;
    int escaped = 0;
    while (*cursor && i + 1 < out_size) {
        if (escaped) {
            out[i++] = *cursor++;
            escaped = 0;
            continue;
        }
        if (*cursor == '\\') {
            escaped = 1;
            cursor++;
            continue;
        }
        if (*cursor == '"') {
            out[i] = '\0';
            return 1;
        }
        out[i++] = *cursor++;
    }
    out[0] = '\0';
    return 0;
}

int sandbox_json_int(const char *line, const char *field, int *out)
{
    if (!out) {
        return 0;
    }
    const char *cursor = field_value(line, field);
    if (!cursor) {
        return 0;
    }
    errno = 0;
    char *end = NULL;
    long value = strtol(cursor, &end, 10);
    if (cursor == end || errno != 0 || value < INT_MIN || value > INT_MAX) {
        return 0;
    }
    *out = (int)value;
    return 1;
}

int sandbox_json_object(const char *line, const char *field, char *out, size_t out_size)
{
    if (!out || out_size == 0) {
        return 0;
    }
    out[0] = '\0';
    const char *cursor = field_value(line, field);
    if (!cursor || *cursor != '{') {
        return 0;
    }
    int depth = 0;
    int in_string = 0;
    int escaped = 0;
    size_t i = 0;
    while (*cursor) {
        if (i + 1 >= out_size) {
            out[0] = '\0';
            return 0;
        }
        char ch = *cursor;
        if (escaped) {
            escaped = 0;
        } else if (ch == '\\' && in_string) {
            escaped = 1;
        } else if (ch == '"') {
            in_string = !in_string;
        } else if (!in_string && ch == '{') {
            depth++;
        } else if (!in_string && ch == '}') {
            depth--;
        }
        out[i++] = *cursor++;
        if (depth == 0) {
            out[i] = '\0';
            return 1;
        }
    }
    out[0] = '\0';
    return 0;
}
