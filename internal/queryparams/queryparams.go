// Package queryparams parses and rebuilds a URL's query string into ordered,
// duplicate-preserving rows for the editor's 查询 tab. The rows are derived from
// and written back to the endpoint, so they need no separate persistence.
package queryparams

import (
	"net/url"
	"strings"
)

// Row is one editable query-string parameter.
type Row struct {
	Key   string
	Value string
}

// Parse splits a URL's query string into rows, preserving order and duplicates.
// Values are URL-decoded.
func Parse(rawURL string) []Row {
	_, query, _ := Split(rawURL)
	if query == "" {
		return nil
	}
	var rows []Row
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		key, val := pair, ""
		if i := strings.IndexByte(pair, '='); i >= 0 {
			key, val = pair[:i], pair[i+1:]
		}
		k, err := url.QueryUnescape(key)
		if err != nil {
			k = key
		}
		v, err := url.QueryUnescape(val)
		if err != nil {
			v = val
		}
		rows = append(rows, Row{Key: k, Value: v})
	}
	return rows
}

// Build rewrites rawURL's query string from rows, keeping the path and the
// fragment untouched. Rows with an empty key and value are dropped.
func Build(rawURL string, rows []Row) string {
	base, _, frag := Split(rawURL)
	var parts []string
	for _, r := range rows {
		if r.Key == "" && r.Value == "" {
			continue
		}
		part := url.QueryEscape(r.Key)
		if r.Value != "" {
			part += "=" + url.QueryEscape(r.Value)
		}
		parts = append(parts, part)
	}
	out := base
	if len(parts) > 0 {
		out += "?" + strings.Join(parts, "&")
	}
	if frag != "" {
		out += "#" + frag
	}
	return out
}

// Split returns the URL without its query, the query, and the fragment.
func Split(rawURL string) (base, query, fragment string) {
	rest := rawURL
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		fragment = rest[i+1:]
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		query = rest[i+1:]
		rest = rest[:i]
	}
	return rest, query, fragment
}
