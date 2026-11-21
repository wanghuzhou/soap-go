package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"soapgo/internal/fileutil"
	"soapgo/internal/model"
)

// Parse decodes an OpenAPI 3.x document and turns it into a native project.
//
// The format is chosen from the file extension (.json means JSON) and, failing
// that, from the first non-space byte, so a .yaml file with JSON content still
// loads. Parsing is strict about the version: a 2.x (Swagger) document would
// otherwise be silently emptied, because paths/operations live elsewhere there.
func Parse(data []byte, filename string) (*model.Project, error) {
	doc, err := decode(data, filename)
	if err != nil {
		return nil, err
	}
	return toProject(doc, filename)
}

// decode reads the document from JSON or YAML into the typed subset.
func decode(data []byte, filename string) (*Document, error) {
	trimmed := bytes.TrimSpace(fileutil.StripBOM(data))
	if len(trimmed) == 0 {
		return nil, errors.New("文件为空")
	}
	var doc Document
	isJSON := strings.EqualFold(filepath.Ext(filename), ".json") || trimmed[0] == '{'
	if isJSON {
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("解析 JSON 失败：%w", err)
		}
	} else {
		if err := yaml.Unmarshal(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("解析 YAML 失败：%w", err)
		}
	}
	if strings.TrimSpace(doc.OpenAPI) == "" {
		return nil, errors.New("不是 OpenAPI 文档：缺少 openapi 字段")
	}
	if !strings.HasPrefix(strings.TrimSpace(doc.OpenAPI), "3.") {
		return nil, fmt.Errorf("暂不支持 OpenAPI %s，请使用 3.x", doc.OpenAPI)
	}
	if len(doc.Paths) == 0 {
		return nil, errors.New("文档里没有任何 paths")
	}
	return &doc, nil
}

// toProject maps a parsed document onto the native tree: one folder per tag and
// one request per operation.
func toProject(doc *Document, filename string) (*model.Project, error) {
	name := strings.TrimSpace(doc.Info.Title)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	if name == "" {
		name = "OpenAPI 项目"
	}
	base := ""
	if len(doc.Servers) > 0 {
		base = strings.TrimRight(strings.TrimSpace(doc.Servers[0].URL), "/")
	}

	project := &model.Project{Name: name, Source: "native"}
	groupIDs := map[string]string{} // tag name -> group id
	paths := make([]string, 0, len(doc.Paths))
	for path := range doc.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		item := doc.Paths[path]
		if item == nil {
			continue
		}
		parameters := append([]Parameter(nil), item.Parameters...)
		for _, mo := range item.operations() {
			op := mo.op
			call := callFromOperation(doc, path, mo.method, op, append(parameters, op.Parameters...), base)
			call.FolderID = folderForTags(project, groupIDs, op.Tags)
			project.Calls = append(project.Calls, call)
		}
	}
	if len(project.Calls) == 0 {
		return nil, errors.New("文档里没有任何接口操作")
	}
	return project, nil
}

// folderForTags returns the folder id for an operation's first tag, creating the
// folder on first use. A tag-less operation stays at the project root.
func folderForTags(project *model.Project, groupIDs map[string]string, tags []string) string {
	for _, raw := range tags {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if id, ok := groupIDs[name]; ok {
			return id
		}
		id := fmt.Sprintf("g%d", len(project.Groups)+1)
		groupIDs[name] = id
		project.Groups = append(project.Groups, model.Group{ID: id, Name: name})
		return id
	}
	return ""
}

// callFromOperation builds one native request from an operation.
func callFromOperation(doc *Document, path, method string, op *Operation, params []Parameter, base string) model.Call {
	title := strings.TrimSpace(op.Summary)
	if title == "" {
		title = strings.TrimSpace(op.OperationID)
	}
	if title == "" {
		title = strings.ToUpper(method) + " " + path
	}
	call := model.Call{
		Title:       title,
		Kind:        model.KindHTTP,
		Method:      strings.ToUpper(method),
		Description: op.Description,
	}

	replaced := path
	query := url.Values{}
	var headers []model.Header
	for _, p := range params {
		switch strings.ToLower(strings.TrimSpace(p.In)) {
		case "path":
			if v := exampleString(p.Example, p.Schema); v != "" {
				replaced = strings.ReplaceAll(replaced, "{"+p.Name+"}", url.PathEscape(v))
			}
		case "query":
			query.Set(p.Name, exampleString(p.Example, p.Schema))
		case "header":
			headers = append(headers, model.Header{Name: p.Name, Value: exampleString(p.Example, p.Schema)})
		}
	}

	endpoint := replaced
	if base != "" {
		if strings.HasPrefix(endpoint, "/") {
			endpoint = base + endpoint
		} else {
			endpoint = base + "/" + endpoint
		}
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	call.Endpoint = endpoint
	call.Headers = headers

	applyRequestBody(&call, doc, op.RequestBody)
	return call
}

// applyRequestBody turns the operation's requestBody into the call's body.
func applyRequestBody(call *model.Call, doc *Document, rb *RequestBody) {
	if rb == nil || len(rb.Content) == 0 {
		return
	}
	name, media := pickContent(rb.Content)
	if media == nil {
		return
	}
	call.ContentType = name
	switch {
	case strings.Contains(name, "json"):
		call.BodyType = model.BodyJSON
		if v := exampleValue(doc, media.Schema, media.Example, 0); v != nil {
			if b, err := json.MarshalIndent(v, "", "  "); err == nil {
				call.Request = string(b)
			}
		} else if s := exampleString(media.Example, media.Schema); s != "" {
			call.Request = s
		}
	case strings.Contains(name, "x-www-form-urlencoded"):
		call.BodyType = model.BodyURLEncoded
		call.BodyFields = fieldsFromSchema(doc, media.Schema)
	case strings.Contains(name, "multipart/form-data"):
		call.BodyType = model.BodyMultipart
		call.BodyFields = fieldsFromSchema(doc, media.Schema)
	case strings.Contains(name, "xml"):
		call.BodyType = model.BodyXML
		call.Request = exampleString(media.Example, media.Schema)
	default:
		call.BodyType = model.BodyText
		call.Request = exampleString(media.Example, media.Schema)
	}
}

// pickContent prefers a JSON media type, then the only entry, then the first in
// sorted order, so a document with several media types still imports predictably.
func pickContent(content map[string]*MediaType) (string, *MediaType) {
	if mt, ok := content["application/json"]; ok {
		return "application/json", mt
	}
	names := make([]string, 0, len(content))
	for name := range content {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.Contains(name, "json") {
			return name, content[name]
		}
	}
	return names[0], content[names[0]]
}

// fieldsFromSchema builds name/value fields from an object schema's properties.
func fieldsFromSchema(doc *Document, schema *Schema) []model.BodyField {
	schema = resolve(doc, schema)
	if schema == nil || len(schema.Properties) == 0 {
		return nil
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]model.BodyField, 0, len(names))
	for _, name := range names {
		out = append(out, model.BodyField{Name: name, Value: exampleString(nil, schema.Properties[name])})
	}
	return out
}

// exampleValue produces a sample value for a schema: the explicit example wins,
// then the schema's own example or first enum value, then a value shaped by its
// type. Depth guards a schema that refers to itself.
func exampleValue(doc *Document, schema *Schema, explicit any, depth int) any {
	if explicit != nil {
		return explicit
	}
	if depth > 8 {
		return nil
	}
	schema = resolve(doc, schema)
	if schema == nil {
		return nil
	}
	if schema.Example != nil {
		return schema.Example
	}
	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}
	switch schema.Type {
	case "object", "":
		if len(schema.Properties) == 0 {
			if schema.Type == "" {
				return nil
			}
			return map[string]any{}
		}
		obj := make(map[string]any, len(schema.Properties))
		names := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			obj[name] = exampleValue(doc, schema.Properties[name], nil, depth+1)
		}
		return obj
	case "array":
		if schema.Items == nil {
			return []any{}
		}
		return []any{exampleValue(doc, schema.Items, nil, depth+1)}
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "string":
		return exampleDateForFormat(schema.Format)
	}
	return nil
}

// exampleString renders a scalar example, falling back to the schema's example.
func exampleString(explicit any, schema *Schema) string {
	v := explicit
	if v == nil && schema != nil {
		v = schema.Example
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return fmt.Sprint(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return fmt.Sprint(v)
}

// exampleDateForFormat gives date/time formats a readable placeholder; other
// strings start empty so the user fills in the real value.
func exampleDateForFormat(format string) string {
	switch strings.ToLower(format) {
	case "date-time":
		return "2026-01-01T00:00:00Z"
	case "date":
		return "2026-01-01"
	}
	return ""
}

// resolve follows a local components/schemas reference; other references are
// returned unchanged (their target is unknown here).
func resolve(doc *Document, schema *Schema) *Schema {
	if schema == nil || schema.Ref == "" || doc == nil || doc.Components == nil {
		return schema
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(schema.Ref, prefix) {
		return schema
	}
	name := strings.TrimPrefix(schema.Ref, prefix)
	if target, ok := doc.Components.Schemas[name]; ok {
		return target
	}
	return schema
}
