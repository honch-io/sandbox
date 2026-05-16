#include <errno.h>
#include <fcntl.h>
#include <honch/honch.h>
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int g_battery_level = 100;

static int sandbox_battery_level(void)
{
    return g_battery_level;
}

static const char *env_or_default(const char *name, const char *fallback)
{
    const char *value = getenv(name);
    return value && value[0] ? value : fallback;
}

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

static void parse_string_field(const char *line, const char *field, char *out, size_t out_size)
{
    char pattern[64];
    snprintf(pattern, sizeof(pattern), "\"%s\"", field);
    const char *cursor = strstr(line, pattern);
    if (!cursor) {
        out[0] = '\0';
        return;
    }
    cursor = strchr(cursor + strlen(pattern), ':');
    if (!cursor) {
        out[0] = '\0';
        return;
    }
    cursor = strchr(cursor, '"');
    if (!cursor) {
        out[0] = '\0';
        return;
    }
    cursor++;
    size_t i = 0;
    while (cursor[i] && cursor[i] != '"' && i + 1 < out_size) {
        out[i] = cursor[i];
        i++;
    }
    out[i] = '\0';
}

static void parse_object_field(const char *line, const char *field, char *out, size_t out_size)
{
    char pattern[64];
    snprintf(pattern, sizeof(pattern), "\"%s\"", field);
    const char *cursor = strstr(line, pattern);
    if (!cursor) {
        out[0] = '\0';
        return;
    }
    cursor = strchr(cursor + strlen(pattern), ':');
    if (!cursor) {
        out[0] = '\0';
        return;
    }
    while (*cursor && *cursor != '{') {
        cursor++;
    }
    if (!*cursor) {
        out[0] = '\0';
        return;
    }
    int depth = 0;
    size_t i = 0;
    while (*cursor && i + 1 < out_size) {
        if (*cursor == '{') {
            depth++;
        } else if (*cursor == '}') {
            depth--;
        }
        out[i++] = *cursor++;
        if (depth == 0) {
            break;
        }
    }
    out[i] = '\0';
}

static void handle_line(honch_client_t *client, const char *line)
{
    if (strstr(line, "\"action\":\"battery\"")) {
        int level = parse_level(line);
        if (level >= 0 && level <= 100) {
            g_battery_level = level;
            printf("{\"ok\":true,\"battery\":%d}\n", g_battery_level);
            fflush(stdout);
        }
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
        honch_status_t status = honch_track(client, event, properties);
        printf("{\"ok\":%s,\"track_status\":%d}\n", status == HONCH_OK ? "true" : "false", status);
        fflush(stdout);
        return;
    }

    if (strstr(line, "\"action\":\"flush\"")) {
        honch_status_t status = honch_flush(client);
        printf("{\"ok\":%s,\"flush_status\":%d}\n", status == HONCH_OK ? "true" : "false", status);
        fflush(stdout);
        return;
    }

    if (strstr(line, "\"action\":\"reset\"")) {
        honch_status_t status = honch_reset(client);
        printf("{\"ok\":%s,\"reset_status\":%d}\n", status == HONCH_OK ? "true" : "false", status);
        fflush(stdout);
        return;
    }
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

int main(void)
{
    const char *endpoint = env_or_default("HONCH_SANDBOX_ENDPOINT", "http://127.0.0.1:18080");
    const char *token = env_or_default("HONCH_SANDBOX_TOKEN", "honch_e2e_test_key");
    const char *control = getenv("HONCH_SANDBOX_CONTROL");
    const char *queue = env_or_default("HONCH_SANDBOX_QUEUE", ".honch-sandbox/c-core-queue");

    honch_config_t config = {
        .api_key = token,
        .endpoint_url = endpoint,
        .device_model = "honch-sandbox-c-core",
        .firmware_version = "sandbox-v1",
        .environment = "sandbox",
        .queue_directory = queue,
        .batch_size = 5,
        .transport_timeout_ms = 3000,
        /* CLI commands drive flushes explicitly so network failures are visible
           to the test operator instead of being hidden by a background worker. */
        .disable_background_flush = 1,
        .battery_callback = sandbox_battery_level,
        .battery_low_threshold = 15,
        .durability_mode = HONCH_DURABILITY_OS_BUFFERED,
    };

    honch_client_t *client = NULL;
    honch_status_t status = honch_init(&client, &config);
    if (status != HONCH_OK) {
        fprintf(stderr, "honch_init failed: %s\n", honch_status_string(status));
        return 1;
    }

    (void)honch_session_start(client, "sandbox-c-core");
    printf("{\"ready\":true,\"adapter\":\"c-core\",\"endpoint\":\"%s\"}\n", endpoint);
    fflush(stdout);

    int control_fd = open_control_fifo(control);
    /* Keep one writer open inside the process. Without this, a FIFO read can
       observe EOF whenever a one-shot CLI control command closes its writer. */
    int control_keepalive_fd = open_control_fifo_keepalive(control);
    struct pollfd fds[2];
    int nfds = 0;
    if (control_fd >= 0) {
        fds[nfds++] = (struct pollfd){.fd = control_fd, .events = POLLIN};
    } else {
        fds[nfds++] = (struct pollfd){.fd = STDIN_FILENO, .events = POLLIN};
    }
    char line[2048];

    for (;;) {
        int ready = poll(fds, (nfds_t)nfds, 1000);
        if (ready < 0) {
            if (errno == EINTR) {
                continue;
            }
            fprintf(stderr, "control poll failed: %s\n", strerror(errno));
            sleep(1);
            continue;
        }
        for (int i = 0; i < nfds; i++) {
            if (!(fds[i].revents & POLLIN)) {
                continue;
            }
            FILE *stream = fdopen(dup(fds[i].fd), "r");
            if (!stream) {
                continue;
            }
            if (fgets(line, sizeof(line), stream)) {
                handle_line(client, line);
            }
            fclose(stream);
        }
    }

    fprintf(stderr, "sandbox harness exiting control loop\n");
    fflush(stderr);
    (void)honch_session_end(client);
    (void)honch_shutdown(client);
    if (control_keepalive_fd >= 0) {
        close(control_keepalive_fd);
    }
    return 0;
}
