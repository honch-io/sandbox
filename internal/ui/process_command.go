package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func ParseProcessCommand(raw string) (string, map[string]any, string, error) {
	action, fields, _, status, err := processCommandSpec(raw)
	if err != nil {
		return "", nil, "", err
	}
	switch action {
	case "flush":
		return action, nil, status, nil
	case "reset":
		return action, nil, status, nil
	case "battery":
		level := fields["level"].(int)
		return action, fields, fmt.Sprintf("battery control has been sent: %d", level), nil
	case "track":
		return action, fields, status, nil
	default:
		return "", nil, "", fmt.Errorf("unknown command %q", action)
	}
}

func ProcessCommandPreview(raw string) (string, error) {
	action, _, preview, _, err := processCommandSpec(raw)
	if err != nil {
		return "", err
	}
	switch action {
	case "flush":
		return "will send: flush", nil
	case "reset":
		return "will send: reset", nil
	case "battery":
		return preview, nil
	case "track":
		return preview, nil
	default:
		return "", fmt.Errorf("unknown command %q", action)
	}
}

func processCommandSpec(raw string) (string, map[string]any, string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, "", "", errors.New("missing command")
	}

	fields := strings.Fields(trimmed)
	switch fields[0] {
	case "flush":
		if len(fields) != 1 {
			return "", nil, "", "", errors.New("flush does not take arguments")
		}
		return "flush", nil, "will send: flush", "flush control has been sent", nil
	case "reset":
		if len(fields) != 1 {
			return "", nil, "", "", errors.New("reset does not take arguments")
		}
		return "reset", nil, "will send: reset", "reset control has been sent", nil
	case "battery":
		if len(fields) < 2 {
			return "", nil, "", "", errors.New("missing battery level")
		}
		levelToken := strings.TrimPrefix(fields[1], "level=")
		levelToken = strings.TrimPrefix(levelToken, "level:")
		level, err := strconv.Atoi(levelToken)
		if err != nil || level < 0 || level > 100 {
			return "", nil, "", "", errors.New("battery level must be between 0 and 100")
		}
		return "battery", map[string]any{"level": level}, fmt.Sprintf("will send: battery %d", level), fmt.Sprintf("battery control has been sent: %d", level), nil
	case "track":
		if len(fields) < 2 {
			return "", nil, "", "", errors.New("missing event name")
		}
		event := fields[1]
		properties := map[string]any{}
		if len(fields) == 2 {
			preview := fmt.Sprintf("will send: track %s {}", event)
			status := fmt.Sprintf("track control has been sent: %s", event)
			return "track", map[string]any{"event": event, "properties": properties}, preview, status, nil
		}
		remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		remainder = strings.TrimSpace(strings.TrimPrefix(remainder, event))
		if strings.HasPrefix(remainder, "{") {
			if err := json.Unmarshal([]byte(remainder), &properties); err != nil || properties == nil {
				return "", nil, "", "", errors.New("properties must be valid JSON")
			}
		} else {
			props, err := parseShorthandProperties(fields[2:])
			if err != nil {
				return "", nil, "", "", err
			}
			properties = props
		}
		preview := fmt.Sprintf("will send: track %s %s", event, renderJSONObject(properties))
		status := fmt.Sprintf("track control has been sent: %s", event)
		return "track", map[string]any{"event": event, "properties": properties}, preview, status, nil
	default:
		return "", nil, "", "", fmt.Errorf("unknown command %q", fields[0])
	}
}

func parseShorthandProperties(tokens []string) (map[string]any, error) {
	props := map[string]any{}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			key, value, ok = strings.Cut(token, ":")
		}
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("properties must use key=value shorthand or JSON")
		}
		props[strings.TrimSpace(key)] = parsePropertyValue(strings.TrimSpace(value))
	}
	return props, nil
}

func parsePropertyValue(value string) any {
	if value == "" {
		return ""
	}
	if json.Valid([]byte(value)) {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed
		}
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	return value
}

func renderJSONObject(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return "{}"
		}
		valueJSON, err := json.Marshal(values[key])
		if err != nil {
			return "{}"
		}
		b.WriteString(string(keyJSON))
		b.WriteByte(':')
		b.WriteString(string(valueJSON))
	}
	b.WriteByte('}')
	return b.String()
}

func commandInputStatus(err error) string {
	switch err.Error() {
	case "missing command":
		return "examples: battery 4, track camera.motion zone=porch"
	case "missing battery level":
		return "enter a battery level, e.g. 4"
	case "battery level must be between 0 and 100":
		return "battery level must be between 0 and 100"
	case "missing event name":
		return "enter an event name, e.g. camera.motion"
	case "flush does not take arguments":
		return "flush takes no arguments"
	case "reset does not take arguments":
		return "reset takes no arguments"
	case "properties must use key=value shorthand or JSON":
		return "use key=value shorthand or JSON"
	case "properties must be valid JSON":
		return "JSON is malformed; try zone=porch instead"
	case "unknown command":
		return "unknown command"
	default:
		if strings.HasPrefix(err.Error(), "unknown command ") {
			return "unknown command"
		}
		return err.Error()
	}
}
