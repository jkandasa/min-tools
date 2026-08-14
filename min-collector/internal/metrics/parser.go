package metrics

import (
	"sort"
	"strings"
)

// Sample is a single parsed Prometheus sample.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  string
}

// LabelString renders the labels as a sorted, comma separated key=value list.
func (s Sample) LabelString() string {
	if len(s.Labels) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(s.Labels))
	for key, value := range s.Labels {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// Parse converts a Prometheus text exposition payload into samples. When names
// is empty every metric is returned, otherwise only the requested ones.
func Parse(payload string, names []string) []Sample {
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}

	var samples []Sample
	for line := range strings.SplitSeq(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		sample, ok := parseLine(line)
		if !ok {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[sample.Name]; !ok {
				continue
			}
		}
		samples = append(samples, sample)
	}
	return samples
}

// parseLine parses `name{label="value",...} value [timestamp]`.
func parseLine(line string) (Sample, bool) {
	braceStart := strings.IndexByte(line, '{')

	// Without labels the line is `name value [timestamp]`.
	if braceStart == -1 {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return Sample{}, false
		}
		return Sample{Name: fields[0], Value: fields[1]}, true
	}

	braceEnd := strings.LastIndexByte(line, '}')
	if braceEnd < braceStart {
		return Sample{}, false
	}

	name := strings.TrimSpace(line[:braceStart])
	if name == "" {
		return Sample{}, false
	}

	fields := strings.Fields(line[braceEnd+1:])
	if len(fields) == 0 {
		return Sample{}, false
	}

	return Sample{
		Name:   name,
		Labels: parseLabels(line[braceStart+1 : braceEnd]),
		Value:  fields[0],
	}, true
}

// parseLabels parses `key="value",key2="value2"`, keeping commas and equal
// signs that appear inside a quoted value.
func parseLabels(input string) map[string]string {
	labels := make(map[string]string)

	var (
		key      string
		buffer   strings.Builder
		inQuotes bool
		escaped  bool
	)

	flush := func() {
		if key != "" {
			labels[key] = strings.TrimSpace(buffer.String())
		}
		key = ""
		buffer.Reset()
	}

	for i := 0; i < len(input); i++ {
		char := input[i]

		if escaped {
			buffer.WriteByte(char)
			escaped = false
			continue
		}

		switch char {
		case '\\':
			if inQuotes {
				escaped = true
				continue
			}
			buffer.WriteByte(char)
		case '"':
			inQuotes = !inQuotes
		case '=':
			if !inQuotes && key == "" {
				key = strings.TrimSpace(buffer.String())
				buffer.Reset()
				continue
			}
			buffer.WriteByte(char)
		case ',':
			if !inQuotes {
				flush()
				continue
			}
			buffer.WriteByte(char)
		default:
			buffer.WriteByte(char)
		}
	}
	flush()

	return labels
}
