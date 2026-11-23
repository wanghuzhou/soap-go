// Package httpfile imports JetBrains HTTP Client files (.http / .rest) as
// native (plain HTTP) projects.
//
// The format is line based: a line starting with "###" opens a new request (the
// rest of that line names it), "Name: value" lines under the request line are
// its headers, a blank line ends the headers and starts the body, and "@name =
// value" lines define variables usable as {{name}}. Everything else — comment
// lines ("#" or "//"), response handlers and scripts ("> {% ... %}", "> ./file",
// ">> file", "< {% ... %}") — is dropped, so only the requests themselves are
// imported.
package httpfile

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"soapgo/internal/fileutil"
	"soapgo/internal/model"
)

// errNoRequests is returned for a file that defines no request at all (for
// example one containing only comments).
var errNoRequests = errors.New("文件里没有任何请求")

// rawRequest is one "###" block before it is turned into a model.Call.
type rawRequest struct {
	title   string
	method  string
	url     string
	headers []headerPair
	body    []string
}

type headerPair struct {
	name  string
	value string
}

// Parser states: outside a request, reading its headers, reading its body.
const (
	stateOut = iota
	stateHeaders
	stateBody
)

var (
	// "@name = value" defines a variable.
	variableRe = regexp.MustCompile(`^@([A-Za-z0-9_.-]+)\s*=\s*(.*)$`)
	// "METHOD URL [HTTP/x]" opens a request.
	requestRe = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|CONNECT)\s+(\S+)`)
	// "Name: value" is a request header.
	headerRe = regexp.MustCompile("^([A-Za-z0-9!#$%&'*+.^_`|~-]+):(.*)$")
	// "# @name foo" (or "// @name foo") names the next request.
	nameRe = regexp.MustCompile(`^(?://|#)\s*@name\s+(\S+)`)
	// "{{name}}" is a variable placeholder.
	templateRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)
)

// Parse turns a .http file into a native project: one request per "###" block.
func Parse(data []byte, filename string) (*model.Project, error) {
	text := string(fileutil.StripBOM(data))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	vars := make(map[string]string)
	var reqs []rawRequest
	var cur *rawRequest
	pending := "" // name from "###" or "# @name" for the next request
	state := stateOut

	finish := func() {
		if cur != nil {
			reqs = append(reqs, *cur)
			cur = nil
		}
		state = stateOut
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		// A "###" line always opens a new request, whatever the current state.
		if strings.HasPrefix(line, "###") {
			finish()
			pending = strings.TrimSpace(strings.TrimPrefix(line, "###"))
			continue
		}
		switch state {
		case stateOut:
			if line == "" {
				continue
			}
			if isComment(line) {
				recordName(line, &pending)
				continue
			}
			if m := variableRe.FindStringSubmatch(line); m != nil {
				vars[m[1]] = strings.TrimSpace(m[2])
				continue
			}
			if m := requestRe.FindStringSubmatch(line); m != nil {
				cur = &rawRequest{title: pending, method: m[1], url: m[2]}
				pending = ""
				state = stateHeaders
			}
		case stateHeaders:
			if line == "" {
				state = stateBody
				continue
			}
			if isHandler(line) {
				finish()
				continue
			}
			if isComment(line) {
				continue
			}
			if m := headerRe.FindStringSubmatch(line); m != nil {
				cur.headers = append(cur.headers, headerPair{
					name:  strings.TrimSpace(m[1]),
					value: strings.TrimSpace(m[2]),
				})
				continue
			}
			// No blank line separated the headers from the body.
			state = stateBody
			cur.body = append(cur.body, raw)
		case stateBody:
			if isHandler(line) {
				finish()
				continue
			}
			// Comments are not part of the body: a note between the body and
			// the next "###", or one just above the body, must not be sent as
			// JSON/XML content.
			if isComment(line) {
				continue
			}
			cur.body = append(cur.body, raw)
		}
	}
	finish()

	if len(reqs) == 0 {
		return nil, errNoRequests
	}
	return build(reqs, vars, filename), nil
}

// isComment reports whether a trimmed line is a comment.
func isComment(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//")
}

// recordName stores a "# @name foo" directive as the next request's title,
// unless one is already pending.
func recordName(line string, pending *string) {
	if *pending != "" {
		return
	}
	if m := nameRe.FindStringSubmatch(line); m != nil {
		*pending = m[1]
	}
}

// isHandler reports whether a trimmed line starts response-handler or script
// code, which marks the end of the request body.
func isHandler(line string) bool {
	switch {
	case strings.HasPrefix(line, ">"),
		strings.HasPrefix(line, "< {%"),
		strings.HasPrefix(line, "<%"),
		strings.HasPrefix(line, "< ./"):
		return true
	}
	return false
}

// build assembles the parsed requests into a native project.
func build(reqs []rawRequest, vars map[string]string, filename string) *model.Project {
	name := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if name == "" {
		name = "HTTP 项目"
	}
	project := &model.Project{Name: name, Source: "native"}
	for _, r := range reqs {
		project.Calls = append(project.Calls, callFrom(r, vars))
	}
	return project
}

// callFrom maps one parsed request onto a native call.
func callFrom(r rawRequest, vars map[string]string) model.Call {
	method := strings.ToUpper(r.method)
	endpoint := substitute(r.url, vars)

	call := model.Call{
		Title:    titleFor(r.title, method, endpoint, vars),
		Kind:     model.KindHTTP,
		Method:   method,
		Endpoint: endpoint,
	}
	for _, h := range r.headers {
		value := substitute(h.value, vars)
		// The media type belongs to the body editor, not the header list, so it
		// does not show up twice; every other header stays as written.
		if strings.EqualFold(h.name, "Content-Type") {
			call.ContentType = value
			continue
		}
		call.Headers = append(call.Headers, model.Header{Name: h.name, Value: value})
	}
	// Drop blank lines around the body (comments were already removed while
	// parsing), so the stored body starts and ends on real content.
	lines := r.body
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	body := substitute(strings.Join(lines, "\n"), vars)
	call.BodyType, call.Request, call.BodyFields = classifyBody(call.ContentType, body)
	return call
}

// titleFor uses the block's own name, or a "METHOD /path" fallback when it has
// none.
func titleFor(raw, method, endpoint string, vars map[string]string) string {
	if title := strings.TrimSpace(substitute(raw, vars)); title != "" {
		return title
	}
	path := endpoint
	if u, err := url.Parse(endpoint); err == nil && u.Path != "" {
		path = u.Path
	}
	return method + " " + path
}

// classifyBody picks the body shape from the content type and, when that is
// missing, from the body's first character.
func classifyBody(contentType, body string) (string, string, []model.BodyField) {
	if strings.TrimSpace(body) == "" {
		return model.BodyNone, "", nil
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		return model.BodyJSON, body, nil
	case strings.Contains(ct, "xml"):
		return model.BodyXML, body, nil
	case strings.Contains(ct, "x-www-form-urlencoded"):
		if fields := parseFormFields(body); len(fields) > 0 {
			return model.BodyURLEncoded, "", fields
		}
		return model.BodyText, body, nil
	}
	trimmed := strings.TrimSpace(body)
	switch {
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		return model.BodyJSON, body, nil
	case strings.HasPrefix(trimmed, "<"):
		return model.BodyXML, body, nil
	}
	return model.BodyText, body, nil
}

// parseFormFields splits an "a=b&c=d" body into ordered name/value fields.
func parseFormFields(body string) []model.BodyField {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	var fields []model.BodyField
	for _, pair := range strings.Split(body, "&") {
		if pair == "" {
			continue
		}
		name, value := pair, ""
		if i := strings.IndexByte(pair, '='); i >= 0 {
			name, value = pair[:i], pair[i+1:]
		}
		if decoded, err := url.QueryUnescape(name); err == nil {
			name = decoded
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		fields = append(fields, model.BodyField{Name: name, Value: value})
	}
	return fields
}

// substitute replaces {{name}} with the value of a defined variable and leaves
// an unknown placeholder untouched, so the user can see what is missing.
func substitute(s string, vars map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return templateRe.ReplaceAllStringFunc(s, func(m string) string {
		name := templateRe.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}
