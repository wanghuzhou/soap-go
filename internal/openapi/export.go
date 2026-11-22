package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"soapgo/internal/model"
)

// Build turns a native project into an OpenAPI 3.0 document. Only native
// projects are supported: a SoapUI or WSDL project describes SOAP services,
// which have no OpenAPI representation here.
func Build(p model.Project) (*Document, error) {
	if !p.CanExportOpenAPI() {
		return nil, errors.New("只有普通 HTTP 项目可以导出 OpenAPI")
	}
	doc := &Document{
		OpenAPI: "3.0.3",
		Info:    Info{Title: p.Name, Version: "1.0.0"},
		Paths:   map[string]*PathItem{},
	}

	groupName := make(map[string]string, len(p.Groups))
	for _, g := range p.Groups {
		groupName[g.ID] = g.Name
	}

	servers := make([]Server, 0, 1)
	seenServer := map[string]bool{}
	// The project host is the server relative request paths are written against.
	if host := model.NormalizeHost(p.Host); host != "" {
		seenServer[host] = true
		servers = append(servers, Server{URL: host})
	}
	tagSeen := map[string]bool{}
	// OpenAPI keys operations by path and method alone, so two requests map to
	// one operation. Refuse the export instead of silently dropping one.
	exported := map[string]string{}
	for _, call := range p.Calls {
		if strings.TrimSpace(call.Endpoint) == "" {
			continue
		}
		path, rawQuery, base, err := splitEndpoint(call.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("请求「%s」的地址无效：%w", call.Title, err)
		}
		method := model.EffectiveMethod(model.KindHTTP, call.Method)
		key := method + " " + path
		if prev, ok := exported[key]; ok {
			return nil, fmt.Errorf("请求「%s」与「%s」都对应 %s %s，OpenAPI 无法表示同一路径的重复方法",
				prev, call.Title, method, path)
		}
		exported[key] = call.Title
		if base != "" && !seenServer[base] {
			seenServer[base] = true
			servers = append(servers, Server{URL: base})
		}
		item := doc.Paths[path]
		if item == nil {
			item = &PathItem{}
			doc.Paths[path] = item
		}
		tag := groupName[call.FolderID]
		if tag != "" && !tagSeen[tag] {
			tagSeen[tag] = true
			doc.Tags = append(doc.Tags, Tag{Name: tag})
		}
		item.setMethod(method, operationFromCall(call, path, rawQuery, tag))
	}
	if len(doc.Paths) == 0 {
		return nil, errors.New("项目里没有带地址的请求，无法导出")
	}
	doc.Servers = servers
	sort.Slice(doc.Tags, func(i, j int) bool { return doc.Tags[i].Name < doc.Tags[j].Name })
	return doc, nil
}

// Marshal renders a document as indented JSON or YAML.
func Marshal(doc *Document, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	case "yaml", "yml", "":
		return yaml.Marshal(doc)
	}
	return nil, fmt.Errorf("未知的导出格式 %q", format)
}

// splitEndpoint separates an endpoint into its path, query string and server
// base. A relative endpoint (no scheme/host) keeps an empty base, so the path
// alone is written — the user can add a server later.
func splitEndpoint(raw string) (path, rawQuery, base string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", "", err
	}
	path = u.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u.Scheme != "" && u.Host != "" {
		base = u.Scheme + "://" + u.Host
	}
	return path, u.RawQuery, base, nil
}

// operationFromCall builds the OpenAPI operation for one native request.
func operationFromCall(call model.Call, path, rawQuery, tag string) *Operation {
	op := &Operation{
		Summary:     call.Title,
		Description: call.Description,
		OperationID: operationID(call.Method, path),
	}
	if tag != "" {
		op.Tags = []string{tag}
	}

	for _, name := range pathParamNames(path) {
		op.Parameters = append(op.Parameters, Parameter{
			Name: name, In: "path", Required: true, Schema: &Schema{Type: "string"},
		})
	}
	if q, err := url.ParseQuery(rawQuery); err == nil {
		keys := make([]string, 0, len(q))
		for name := range q {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			op.Parameters = append(op.Parameters, Parameter{
				Name: name, In: "query", Example: q.Get(name), Schema: &Schema{Type: "string"},
			})
		}
	}
	for _, h := range call.Headers {
		if !exportableHeader(h.Name) {
			continue
		}
		op.Parameters = append(op.Parameters, Parameter{
			Name: h.Name, In: "header", Example: h.Value, Schema: &Schema{Type: "string"},
		})
	}

	op.RequestBody = requestBodyFromCall(call)
	op.Responses = responsesFromCall(call)
	return op
}

// requestBodyFromCall builds the request body, or nil when the request has none.
func requestBodyFromCall(call model.Call) *RequestBody {
	bodyType := model.NormalizeBodyType(call.BodyType)
	if bodyType == "" {
		bodyType = model.InferBodyType(call.ContentType, call.Request)
	}
	switch bodyType {
	case model.BodyNone, "":
		return nil
	}
	contentType := strings.TrimSpace(call.ContentType)
	if contentType == "" {
		contentType = model.BodyTypeContentType(bodyType)
	}
	if contentType == "" {
		contentType = "application/json"
	}

	media := &MediaType{}
	switch bodyType {
	case model.BodyJSON:
		var value any
		if err := json.Unmarshal([]byte(call.Request), &value); err == nil {
			media.Example = value
			media.Schema = schemaFromValue(value, 0)
		} else {
			media.Schema = &Schema{Type: "string"}
			if call.Request != "" {
				media.Example = call.Request
			}
		}
	case model.BodyURLEncoded, model.BodyMultipart:
		obj := make(map[string]any, len(call.BodyFields))
		props := make(map[string]*Schema, len(call.BodyFields))
		for _, f := range call.BodyFields {
			obj[f.Name] = f.Value
			if f.Type == "file" {
				props[f.Name] = &Schema{Type: "string", Format: "binary"}
			} else {
				props[f.Name] = &Schema{Type: "string"}
			}
		}
		media.Schema = &Schema{Type: "object", Properties: props}
		media.Example = obj
	case model.BodyBinary:
		if call.BodyFile == "" {
			return nil
		}
		media.Schema = &Schema{Type: "string", Format: "binary"}
	default:
		media.Schema = &Schema{Type: "string"}
		if call.Request != "" {
			media.Example = call.Request
		}
	}
	return &RequestBody{Required: true, Content: map[string]*MediaType{contentType: media}}
}

// responsesFromCall builds the responses map. A saved response becomes a real
// example; otherwise only an empty 200 remains.
func responsesFromCall(call model.Call) map[string]*Response {
	out := map[string]*Response{}
	if saved := call.Response; saved != nil && !saved.Truncated && saved.Body != "" {
		status := saved.Status
		if status == 0 {
			status = 200
		}
		description := saved.StatusText
		if strings.TrimSpace(description) == "" {
			description = "成功"
		}
		contentType := headerValue(saved.Headers, "Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
		media := &MediaType{}
		var value any
		if err := json.Unmarshal([]byte(saved.Body), &value); err == nil {
			media.Example = value
			media.Schema = schemaFromValue(value, 0)
		} else {
			media.Schema = &Schema{Type: "string"}
			media.Example = saved.Body
		}
		out[strconv.Itoa(status)] = &Response{
			Description: description,
			Content:     map[string]*MediaType{contentType: media},
		}
		return out
	}
	out["200"] = &Response{Description: "成功"}
	return out
}

// schemaFromValue infers a JSON schema from an example value.
func schemaFromValue(value any, depth int) *Schema {
	if depth > 8 {
		return &Schema{}
	}
	switch v := value.(type) {
	case nil:
		return &Schema{}
	case bool:
		return &Schema{Type: "boolean", Example: v}
	case string:
		return &Schema{Type: "string", Example: v}
	case float64:
		if v == float64(int64(v)) {
			return &Schema{Type: "integer", Example: int64(v)}
		}
		return &Schema{Type: "number", Example: v}
	case json.Number:
		return &Schema{Type: "number", Example: v}
	case []any:
		schema := &Schema{Type: "array"}
		if len(v) > 0 {
			schema.Items = schemaFromValue(v[0], depth+1)
		}
		return schema
	case map[string]any:
		props := make(map[string]*Schema, len(v))
		for name, item := range v {
			props[name] = schemaFromValue(item, depth+1)
		}
		return &Schema{Type: "object", Properties: props}
	}
	return &Schema{}
}

// operationID makes a stable camelCase id from the method and path.
func operationID(method, path string) string {
	parts := []string{strings.ToLower(strings.TrimSpace(method))}
	for _, seg := range strings.Split(path, "/") {
		seg = strings.Trim(seg, "{}")
		seg = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, seg)
		if seg != "" {
			parts = append(parts, strings.ToUpper(seg[:1])+seg[1:])
		}
	}
	return strings.Join(parts, "")
}

// pathParamNames returns the {name} placeholders of a path.
func pathParamNames(path string) []string {
	var names []string
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			break
		}
		name := strings.TrimSpace(path[start+1 : start+end])
		if name != "" {
			names = append(names, name)
		}
		path = path[start+end+1:]
	}
	return names
}

// exportableHeader reports whether a request header should be written as a
// parameter. Content-Type and credentials are already represented elsewhere.
func exportableHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "content-type", "authorization", "cookie", "content-length", "host":
		return false
	}
	return true
}

// headerValue looks up a header case-insensitively.
func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}
