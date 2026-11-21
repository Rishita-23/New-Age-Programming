package config

import (
	"fmt"
	"strings"
)

func parseYAML(src string) (map[string]any, error) {
	out := map[string]any{}
	var listKey string
	for i, raw := range strings.Split(src, "\n") {
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if listKey == "" {
				return nil, fmt.Errorf("line %d: list item without a key", i+1)
			}
			item := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
			out[listKey] = append(out[listKey].([]string), item)
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			return nil, fmt.Errorf("line %d: unsupported indentation", i+1)
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key: value", i+1)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", i+1, key)
		}
		switch {
		case val == "":
			out[key] = []string{}
			listKey = key
		case strings.HasPrefix(val, "["):
			if !strings.HasSuffix(val, "]") {
				return nil, fmt.Errorf("line %d: unterminated list", i+1)
			}
			inner := strings.TrimSpace(val[1 : len(val)-1])
			items := []string{}
			if inner != "" {
				for _, p := range strings.Split(inner, ",") {
					items = append(items, unquote(strings.TrimSpace(p)))
				}
			}
			out[key] = items
			listKey = ""
		default:
			out[key] = unquote(val)
			listKey = ""
		}
	}
	return out, nil
}

func stripComment(s string) string {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
