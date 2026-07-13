// Package yamlite implements a tiny, dependency-free parser for the subset
// of YAML that Aegis policy files use: nested maps, lists, comments,
// strings/ints/floats/bools and inline [a, b, c] arrays.
//
// It exists so that `go run .` works out of the box with zero external
// modules. If you need full YAML (anchors, multiline strings, etc.) swap
// this package for gopkg.in/yaml.v3 — LoadPolicy is the only caller.
package yamlite

import (
	"strconv"
	"strings"
)

type line struct {
	indent int
	text   string
}

// Parse turns raw YAML-subset text into a generic tree of
// map[string]interface{}, []interface{}, string, int, float64 and bool.
func Parse(data string) interface{} {
	lines := tokenize(data)
	if len(lines) == 0 {
		return map[string]interface{}{}
	}
	pos := 0
	return parseBlock(lines, &pos, lines[0].indent)
}

func tokenize(data string) []line {
	rawLines := strings.Split(data, "\n")
	out := make([]line, 0, len(rawLines))
	for _, raw := range strings.Split(data, "\n") {
		l := raw
		if idx := strings.Index(l, "#"); idx >= 0 {
			l = l[:idx]
		}
		trimmed := strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		indent := 0
		for indent < len(trimmed) && trimmed[indent] == ' ' {
			indent++
		}
		out = append(out, line{indent: indent, text: strings.TrimSpace(trimmed)})
	}
	return out
}

func parseBlock(lines []line, pos *int, indent int) interface{} {
	if *pos >= len(lines) || lines[*pos].indent < indent {
		return nil
	}
	if isListItem(lines[*pos].text) {
		return parseList(lines, pos, indent)
	}
	return parseMap(lines, pos, indent)
}

func isListItem(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ")
}

func parseList(lines []line, pos *int, indent int) []interface{} {
	var result []interface{}
	for *pos < len(lines) && lines[*pos].indent == indent && isListItem(lines[*pos].text) {
		rest := strings.TrimSpace(strings.TrimPrefix(lines[*pos].text, "-"))
		*pos++
		switch {
		case rest == "":
			result = append(result, parseBlock(lines, pos, indent+2))
		case strings.Contains(rest, ":"):
			m := map[string]interface{}{}
			k, v := splitKV(rest)
			assign(lines, pos, indent+2, m, k, v)
			for *pos < len(lines) && lines[*pos].indent == indent+2 && !isListItem(lines[*pos].text) {
				k2, v2 := splitKV(lines[*pos].text)
				*pos++
				assign(lines, pos, indent+4, m, k2, v2)
			}
			result = append(result, m)
		default:
			result = append(result, parseScalar(rest))
		}
	}
	return result
}

func parseMap(lines []line, pos *int, indent int) map[string]interface{} {
	m := map[string]interface{}{}
	for *pos < len(lines) && lines[*pos].indent == indent {
		k, v := splitKV(lines[*pos].text)
		*pos++
		assign(lines, pos, indent+2, m, k, v)
	}
	return m
}

// assign fills m[k]. If v is empty, the value lives in a nested block that
// follows on subsequent, deeper-indented lines.
func assign(lines []line, pos *int, childIndent int, m map[string]interface{}, k, v string) {
	if v != "" {
		m[k] = parseScalar(v)
		return
	}
	if *pos < len(lines) && lines[*pos].indent >= childIndent {
		m[k] = parseBlock(lines, pos, lines[*pos].indent)
	} else {
		m[k] = nil
	}
}

func splitKV(s string) (k, v string) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
}

func parseScalar(s string) interface{} {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
		if strings.TrimSpace(inner) == "" {
			return []interface{}{}
		}
		var arr []interface{}
		for _, p := range strings.Split(inner, ",") {
			arr = append(arr, parseScalar(strings.TrimSpace(p)))
		}
		return arr
	}
	return s
}
