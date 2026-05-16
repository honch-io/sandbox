#include "sandbox_json.h"

#include <ctype.h>
#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct json_slice {
    const char *start;
    const char *end;
} json_slice_t;

static void skip_ws(const char **cursor)
{
    while (**cursor && isspace((unsigned char)**cursor)) {
        (*cursor)++;
    }
}

static int is_hex(char ch)
{
    return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F');
}

static int parse_value(const char **cursor, json_slice_t *slice);

static int parse_string_token(const char **cursor)
{
    if (**cursor != '"') {
        return 0;
    }
    (*cursor)++;
    while (**cursor) {
        unsigned char ch = (unsigned char)**cursor;
        if (ch < 0x20u) {
            return 0;
        }
        if (ch == '"') {
            (*cursor)++;
            return 1;
        }
        if (ch != '\\') {
            (*cursor)++;
            continue;
        }
        (*cursor)++;
        switch (**cursor) {
        case '"':
        case '\\':
        case '/':
        case 'b':
        case 'f':
        case 'n':
        case 'r':
        case 't':
            (*cursor)++;
            break;
        case 'u':
            (*cursor)++;
            for (int i = 0; i < 4; i++) {
                if (!is_hex((*cursor)[i])) {
                    return 0;
                }
            }
            *cursor += 4;
            break;
        default:
            return 0;
        }
    }
    return 0;
}

static int parse_literal(const char **cursor, const char *literal)
{
    size_t length = strlen(literal);
    if (strncmp(*cursor, literal, length) != 0) {
        return 0;
    }
    *cursor += length;
    return 1;
}

static int parse_number(const char **cursor)
{
    const char *start = *cursor;
    if (**cursor == '-') {
        (*cursor)++;
    }
    if (**cursor == '0') {
        (*cursor)++;
    } else if (**cursor >= '1' && **cursor <= '9') {
        while (isdigit((unsigned char)**cursor)) {
            (*cursor)++;
        }
    } else {
        *cursor = start;
        return 0;
    }
    if (**cursor == '.') {
        (*cursor)++;
        if (!isdigit((unsigned char)**cursor)) {
            *cursor = start;
            return 0;
        }
        while (isdigit((unsigned char)**cursor)) {
            (*cursor)++;
        }
    }
    if (**cursor == 'e' || **cursor == 'E') {
        (*cursor)++;
        if (**cursor == '+' || **cursor == '-') {
            (*cursor)++;
        }
        if (!isdigit((unsigned char)**cursor)) {
            *cursor = start;
            return 0;
        }
        while (isdigit((unsigned char)**cursor)) {
            (*cursor)++;
        }
    }
    return 1;
}

static int parse_array(const char **cursor)
{
    if (**cursor != '[') {
        return 0;
    }
    (*cursor)++;
    skip_ws(cursor);
    if (**cursor == ']') {
        (*cursor)++;
        return 1;
    }
    for (;;) {
        if (!parse_value(cursor, NULL)) {
            return 0;
        }
        skip_ws(cursor);
        if (**cursor == ']') {
            (*cursor)++;
            return 1;
        }
        if (**cursor != ',') {
            return 0;
        }
        (*cursor)++;
        skip_ws(cursor);
    }
}

static int parse_object(const char **cursor)
{
    if (**cursor != '{') {
        return 0;
    }
    (*cursor)++;
    skip_ws(cursor);
    if (**cursor == '}') {
        (*cursor)++;
        return 1;
    }
    for (;;) {
        if (!parse_string_token(cursor)) {
            return 0;
        }
        skip_ws(cursor);
        if (**cursor != ':') {
            return 0;
        }
        (*cursor)++;
        if (!parse_value(cursor, NULL)) {
            return 0;
        }
        skip_ws(cursor);
        if (**cursor == '}') {
            (*cursor)++;
            return 1;
        }
        if (**cursor != ',') {
            return 0;
        }
        (*cursor)++;
        skip_ws(cursor);
    }
}

static int parse_value(const char **cursor, json_slice_t *slice)
{
    skip_ws(cursor);
    const char *start = *cursor;
    int ok = 0;
    switch (**cursor) {
    case '{':
        ok = parse_object(cursor);
        break;
    case '[':
        ok = parse_array(cursor);
        break;
    case '"':
        ok = parse_string_token(cursor);
        break;
    case 't':
        ok = parse_literal(cursor, "true");
        break;
    case 'f':
        ok = parse_literal(cursor, "false");
        break;
    case 'n':
        ok = parse_literal(cursor, "null");
        break;
    default:
        ok = parse_number(cursor);
        break;
    }
    if (!ok) {
        return 0;
    }
    if (slice) {
        slice->start = start;
        slice->end = *cursor;
    }
    return 1;
}

static int copy_json_string(json_slice_t slice, char *out, size_t out_size)
{
    if (!out || out_size == 0 || slice.start >= slice.end || *slice.start != '"') {
        return 0;
    }
    out[0] = '\0';
    const char *cursor = slice.start + 1;
    const char *end = slice.end - 1;
    size_t used = 0;
    while (cursor < end) {
        char ch = *cursor++;
        if (ch == '\\') {
            if (cursor >= end) {
                out[0] = '\0';
                return 0;
            }
            ch = *cursor++;
            switch (ch) {
            case '"':
            case '\\':
            case '/':
                break;
            case 'b':
                ch = '\b';
                break;
            case 'f':
                ch = '\f';
                break;
            case 'n':
                ch = '\n';
                break;
            case 'r':
                ch = '\r';
                break;
            case 't':
                ch = '\t';
                break;
            case 'u':
                if (end - cursor < 4) {
                    out[0] = '\0';
                    return 0;
                }
                cursor += 4;
                ch = '?';
                break;
            default:
                out[0] = '\0';
                return 0;
            }
        }
        if (used + 1 >= out_size) {
            out[0] = '\0';
            return 0;
        }
        out[used++] = ch;
    }
    out[used] = '\0';
    return 1;
}

static int string_token_equals(json_slice_t slice, const char *field)
{
    char key[128];
    return copy_json_string(slice, key, sizeof(key)) && strcmp(key, field) == 0;
}

static int top_level_field_value(const char *line, const char *field, json_slice_t *out)
{
    if (!line || !field || !out) {
        return 0;
    }
    const char *cursor = line;
    skip_ws(&cursor);
    if (*cursor != '{') {
        return 0;
    }
    cursor++;
    skip_ws(&cursor);
    int found = 0;
    json_slice_t value = {0};
    if (*cursor != '}') {
        for (;;) {
            const char *key_start = cursor;
            if (!parse_string_token(&cursor)) {
                return 0;
            }
            json_slice_t key = {.start = key_start, .end = cursor};
            skip_ws(&cursor);
            if (*cursor != ':') {
                return 0;
            }
            cursor++;
            json_slice_t candidate = {0};
            if (!parse_value(&cursor, &candidate)) {
                return 0;
            }
            if (!found && string_token_equals(key, field)) {
                value = candidate;
                found = 1;
            }
            skip_ws(&cursor);
            if (*cursor == '}') {
                break;
            }
            if (*cursor != ',') {
                return 0;
            }
            cursor++;
            skip_ws(&cursor);
        }
    }
    cursor++;
    skip_ws(&cursor);
    if (*cursor != '\0' || !found) {
        return 0;
    }
    *out = value;
    return 1;
}

int sandbox_json_string(const char *line, const char *field, char *out, size_t out_size)
{
    if (!out || out_size == 0) {
        return 0;
    }
    out[0] = '\0';
    json_slice_t value = {0};
    if (!top_level_field_value(line, field, &value) || value.start >= value.end || *value.start != '"') {
        return 0;
    }
    return copy_json_string(value, out, out_size);
}

int sandbox_json_int(const char *line, const char *field, int *out)
{
    if (!out) {
        return 0;
    }
    json_slice_t value = {0};
    if (!top_level_field_value(line, field, &value)) {
        return 0;
    }
    errno = 0;
    char *end = NULL;
    long parsed = strtol(value.start, &end, 10);
    if (value.start == end || end != value.end || errno != 0 || parsed < INT_MIN || parsed > INT_MAX) {
        return 0;
    }
    *out = (int)parsed;
    return 1;
}

int sandbox_json_object(const char *line, const char *field, char *out, size_t out_size)
{
    if (!out || out_size == 0) {
        return 0;
    }
    out[0] = '\0';
    json_slice_t value = {0};
    if (!top_level_field_value(line, field, &value) || value.start >= value.end || *value.start != '{') {
        return 0;
    }
    size_t length = (size_t)(value.end - value.start);
    if (length + 1 > out_size) {
        return 0;
    }
    memcpy(out, value.start, length);
    out[length] = '\0';
    return 1;
}
