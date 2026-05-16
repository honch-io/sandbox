#include "sandbox_control.h"

#include <errno.h>
#include <fcntl.h>
#include <honch/honch.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int parse_level(const char *line)
{
    const char *level = strstr(line, "\"level\"");
    if (!level) {
        return -1;
    }
    while (*level && (*level < '0' || *level > '9')) {
        level++;
    }
    if (!*level) {
        return -1;
    }
    return atoi(level);
}

static const char *json_field_value(const char *line, const char *field)
{
    char pattern[64];
    snprintf(pattern, sizeof(pattern), "\"%s\"", field);
    const char *cursor = strstr(line, pattern);
    if (!cursor) {
        return NULL;
    }
    cursor = strchr(cursor + strlen(pattern), ':');
    if (!cursor) {
        return NULL;
    }
    cursor++;
    while (*cursor == ' ' || *cursor == '\t') {
        cursor++;
    }
    return cursor;
}

static void parse_string_field(const char *line, const char *field, char *out, size_t out_size)
{
    const char *cursor = json_field_value(line, field);
    if (!cursor || *cursor != '"') {
        out[0] = '\0';
        return;
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
            break;
        }
        out[i++] = *cursor++;
    }
    out[i] = '\0';
}

static void parse_object_field(const char *line, const char *field, char *out, size_t out_size)
{
    const char *cursor = json_field_value(line, field);
    if (!cursor || *cursor != '{') {
        out[0] = '\0';
        return;
    }
    int depth = 0;
    int in_string = 0;
    int escaped = 0;
    size_t i = 0;
    while (*cursor && i + 1 < out_size) {
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
            break;
        }
    }
    out[i] = '\0';
}

static void print_status(const char *name, honch_status_t status)
{
    printf("{\"ok\":%s,\"%s_status\":%d}\n", status == HONCH_OK ? "true" : "false", name, status);
    fflush(stdout);
}

static void handle_line(sandbox_app_t *app, const char *line)
{
    if (strstr(line, "\"action\":\"battery\"")) {
        int level = parse_level(line);
        if (level >= 0 && level <= 100) {
            sandbox_app_set_battery(app, level);
            printf("{\"ok\":true,\"battery\":%d}\n", app->battery_level);
        } else {
            printf("{\"ok\":false,\"error\":\"invalid_battery\"}\n");
        }
        fflush(stdout);
        return;
    }

    if (strstr(line, "\"action\":\"track\"")) {
        char event[128];
        char properties[1024];
        parse_string_field(line, "event", event, sizeof(event));
        parse_object_field(line, "properties", properties, sizeof(properties));
        if (!event[0]) {
            strcpy(event, "sandbox.event");
        }
        if (!properties[0]) {
            strcpy(properties, "{}");
        }
        print_status("track", sandbox_app_track(app, event, properties));
        return;
    }

    if (strstr(line, "\"action\":\"flush\"")) {
        print_status("flush", sandbox_app_flush(app));
        return;
    }

    if (strstr(line, "\"action\":\"reset\"")) {
        print_status("reset", sandbox_app_reset(app));
        return;
    }

    printf("{\"ok\":false,\"error\":\"unknown_action\"}\n");
    fflush(stdout);
}

static int open_control_fifo(const char *path)
{
    if (!path || !path[0]) {
        return -1;
    }
    int fd = open(path, O_RDONLY | O_NONBLOCK);
    if (fd < 0) {
        fprintf(stderr, "failed to open control fifo %s: %s\n", path, strerror(errno));
    }
    return fd;
}

static int open_control_fifo_keepalive(const char *path)
{
    if (!path || !path[0]) {
        return -1;
    }
    return open(path, O_WRONLY | O_NONBLOCK);
}

int sandbox_control_run(sandbox_app_t *app, const char *control_path)
{
    int control_fd = open_control_fifo(control_path);
    /* Keep one writer open inside the process. Without this, a FIFO read can
       observe EOF whenever a one-shot CLI control command closes its writer. */
    int control_keepalive_fd = open_control_fifo_keepalive(control_path);
    struct pollfd fds[1];
    if (control_fd >= 0) {
        fds[0] = (struct pollfd){.fd = control_fd, .events = POLLIN};
    } else {
        fds[0] = (struct pollfd){.fd = STDIN_FILENO, .events = POLLIN};
    }

    char line[2048];
    char control_buffer[4096];
    size_t control_buffer_len = 0;

    for (;;) {
        int ready = poll(fds, 1, 1000);
        if (ready < 0) {
            if (errno == EINTR) {
                continue;
            }
            fprintf(stderr, "control poll failed: %s\n", strerror(errno));
            sleep(1);
            continue;
        }
        if (!(fds[0].revents & POLLIN)) {
            continue;
        }
        for (;;) {
            ssize_t n = read(fds[0].fd, control_buffer + control_buffer_len, sizeof(control_buffer) - control_buffer_len - 1);
            if (n < 0) {
                if (errno == EAGAIN || errno == EWOULDBLOCK) {
                    break;
                }
                fprintf(stderr, "control read failed: %s\n", strerror(errno));
                break;
            }
            if (n == 0) {
                break;
            }
            control_buffer_len += (size_t)n;
            control_buffer[control_buffer_len] = '\0';
            char *start = control_buffer;
            char *newline = NULL;
            while ((newline = strchr(start, '\n')) != NULL) {
                *newline = '\0';
                snprintf(line, sizeof(line), "%s", start);
                handle_line(app, line);
                start = newline + 1;
            }
            size_t remaining = strlen(start);
            memmove(control_buffer, start, remaining);
            control_buffer_len = remaining;
            control_buffer[control_buffer_len] = '\0';
            if (control_buffer_len == sizeof(control_buffer) - 1) {
                fprintf(stderr, "control line too long, dropping buffered input\n");
                control_buffer_len = 0;
                control_buffer[0] = '\0';
            }
        }
    }

    if (control_keepalive_fd >= 0) {
        close(control_keepalive_fd);
    }
    return 0;
}
