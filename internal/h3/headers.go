package h3

import (
	"net/http"
	"sort"
	"strings"
)

func EncodeHeaders(headers map[string]string) []byte {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+":"+headers[k])
	}
	return []byte(strings.Join(lines, "\n"))
}

func DecodeHeaders(block []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(block), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			parts := strings.SplitN(line[1:], ":", 2)
			if len(parts) != 2 {
				continue
			}
			out[":"+parts[0]] = parts[1]
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		out[parts[0]] = parts[1]
	}
	return out
}

func ToHTTPHeader(input map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range input {
		if strings.HasPrefix(k, ":") {
			continue
		}
		h.Set(k, v)
	}
	return h
}
