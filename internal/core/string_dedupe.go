package core

import "strings"

func CollectUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = AppendUniqueString(out, seen, v)
	}
	return out
}

func AppendUniqueString(dst []string, seen map[string]struct{}, value string) []string {
	if _, ok := seen[value]; ok {
		return dst
	}
	seen[value] = struct{}{}
	return append(dst, value)
}
