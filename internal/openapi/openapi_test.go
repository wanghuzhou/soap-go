package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"soapgo/internal/model"
)

const sampleJSON = `{
  "openapi": "3.0.1",
  "info": {"title": "默认模块", "version": "1.0.0"},
  "tags": [{"name": "User"}],
  "paths": {
    "/user": {
      "get": {
        "summary": "用户列表",
        "tags": ["User"],
        "parameters": [
          {"name": "username", "in": "query", "example": "zhang", "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "ok"}}
      },
      "post": {
        "summary": "新增用户",
        "description": "新增用户。",
        "tags": ["User"],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UserCreateRequest"}}}},
        "responses": {"200": {"description": "ok"}}
      }
    },
    "/user/{id}": {
      "get": {
        "summary": "用户详情",
        "tags": ["User"],
        "parameters": [
          {"name": "id", "in": "path", "required": true, "example": "1", "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "ok"}}
      }
    }
  },
  "components": {"schemas": {
    "UserCreateRequest": {
      "type": "object",
      "properties": {
        "username": {"type": "string", "example": "zhangsan"},
        "password": {"type": "string", "example": "123456"}
      },
      "required": ["username"]
    }
  }}
}`

const sampleYAML = `openapi: 3.0.1
info:
  title: 默认模块
  version: 1.0.0
tags:
  - name: User
paths:
  /user:
    get:
      summary: 用户列表
      tags: [User]
      parameters:
        - name: username
          in: query
          example: zhang
          schema: {type: string}
      responses:
        '200':
          description: ok
  /user/{id}:
    get:
      summary: 用户详情
      tags: [User]
      parameters:
        - name: id
          in: path
          required: true
          example: '1'
          schema: {type: string}
      responses:
        '200':
          description: ok
`

// TestParseJSON checks the JSON path: tags become folders, operations become
// requests, query/path examples land in the endpoint and a $ref body is
// rendered from its component schema.
func TestParseJSON(t *testing.T) {
	p, err := Parse([]byte(sampleJSON), "a.openapi.json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "默认模块" || p.Source != "native" {
		t.Fatalf("project = %q/%q", p.Name, p.Source)
	}
	if len(p.Groups) != 1 || p.Groups[0].Name != "User" {
		t.Fatalf("groups = %+v", p.Groups)
	}
	if len(p.Calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(p.Calls))
	}
	byTitle := map[string]model.Call{}
	for _, c := range p.Calls {
		byTitle[c.Title] = c
	}
	if got := byTitle["用户列表"].Endpoint; got != "/user?username=zhang" {
		t.Errorf("list endpoint = %q", got)
	}
	if got := byTitle["用户详情"].Endpoint; got != "/user/1" {
		t.Errorf("detail endpoint = %q", got)
	}
	create := byTitle["新增用户"]
	if create.BodyType != model.BodyJSON || create.ContentType != "application/json" {
		t.Errorf("create body = %q/%q", create.BodyType, create.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(create.Request), &body); err != nil {
		t.Fatalf("create body is not JSON: %v (%q)", err, create.Request)
	}
	if body["username"] != "zhangsan" {
		t.Errorf("create body username = %v", body["username"])
	}
	if create.Description != "新增用户。" {
		t.Errorf("description = %q", create.Description)
	}
	for _, c := range p.Calls {
		if c.FolderID != p.Groups[0].ID {
			t.Errorf("call %q folder = %q", c.Title, c.FolderID)
		}
	}
}

// TestParseJSONWithBOM checks a document saved by an editor that prefixes a
// UTF-8 BOM still imports, as the other import paths already allow.
func TestParseJSONWithBOM(t *testing.T) {
	plain := `{"openapi":"3.0.3","info":{"title":"带 BOM","version":"1"},` +
		`"paths":{"/a":{"get":{"responses":{"200":{"description":"ok"}}}}}}`
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(plain)...)
	p, err := Parse(data, "a.openapi.json")
	if err != nil {
		t.Fatalf("Parse with BOM: %v", err)
	}
	if len(p.Calls) != 1 || p.Calls[0].Endpoint != "/a" {
		t.Fatalf("calls = %+v, want the /a request", p.Calls)
	}
}

// TestParseYAML checks the YAML path produces the same shape.
func TestParseYAML(t *testing.T) {
	p, err := Parse([]byte(sampleYAML), "b.openapi.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(p.Calls))
	}
	if len(p.Groups) != 1 || p.Groups[0].Name != "User" {
		t.Fatalf("groups = %+v", p.Groups)
	}
	for _, c := range p.Calls {
		if c.Title == "用户详情" && c.Endpoint != "/user/1" {
			t.Errorf("detail endpoint = %q", c.Endpoint)
		}
	}
}

// TestParseRejectsSwagger makes sure a 2.x document fails loudly instead of
// importing an empty project.
func TestParseRejectsSwagger(t *testing.T) {
	if _, err := Parse([]byte(`{"swagger":"2.0","info":{"title":"x"}}`), "old.json"); err == nil {
		t.Fatal("expected an error for a Swagger 2.0 document")
	}
}

// TestBuildRoundTrip exports a native project and parses the result back.
func TestBuildRoundTrip(t *testing.T) {
	project := model.Project{
		Name:   "示例",
		Source: "native",
		Groups: []model.Group{{ID: "g1", Name: "User"}},
		Calls: []model.Call{
			{Title: "列表", Method: "GET", Kind: model.KindHTTP, Endpoint: "https://api.test/user?username=z", FolderID: "g1"},
			{Title: "创建", Method: "POST", Kind: model.KindHTTP, Endpoint: "https://api.test/user",
				Description: "创建用户", BodyType: model.BodyJSON, ContentType: "application/json",
				Request: `{"name":"a"}`, FolderID: "g1"},
		},
	}
	doc, err := Build(project)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if doc.OpenAPI != "3.0.3" || doc.Info.Title != "示例" {
		t.Fatalf("doc = %+v", doc.Info)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "https://api.test" {
		t.Fatalf("servers = %+v", doc.Servers)
	}
	item := doc.Paths["/user"]
	if item == nil || item.Get == nil || item.Post == nil {
		t.Fatalf("paths = %+v", doc.Paths)
	}
	if item.Post.RequestBody == nil {
		t.Fatal("POST lost its request body")
	}
	if item.Post.Description != "创建用户" {
		t.Errorf("description = %q", item.Post.Description)
	}

	for _, format := range []string{"json", "yaml"} {
		data, err := Marshal(doc, format)
		if err != nil {
			t.Fatalf("Marshal %s: %v", format, err)
		}
		back, err := Parse(data, "x."+format)
		if err != nil {
			t.Fatalf("re-parse %s: %v", format, err)
		}
		if len(back.Calls) != 2 {
			t.Errorf("%s: calls = %d, want 2", format, len(back.Calls))
		}
		for _, c := range back.Calls {
			if !strings.HasPrefix(c.Endpoint, "https://api.test/user") {
				t.Errorf("%s: endpoint = %q", format, c.Endpoint)
			}
		}
	}
}

// TestBuildUsesProjectHostAsServer checks that relative request paths are
// exported against the project host, so importing the document back yields the
// joined absolute address.
func TestBuildUsesProjectHostAsServer(t *testing.T) {
	project := model.Project{
		Name: "hosted", Source: "native", Host: "http://localhost/projectA",
		Calls: []model.Call{{Title: "列表", Method: "GET", Kind: model.KindHTTP, Endpoint: "/a/b/list"}},
	}
	doc, err := Build(project)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "http://localhost/projectA" {
		t.Fatalf("servers = %+v, want the project host", doc.Servers)
	}
	if doc.Paths["/a/b/list"] == nil {
		t.Fatalf("paths = %+v, want /a/b/list", doc.Paths)
	}

	data, err := Marshal(doc, "json")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := Parse(data, "x.json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := back.Calls[0].Endpoint, "http://localhost/projectA/a/b/list"; got != want {
		t.Fatalf("re-imported endpoint = %q, want %q", got, want)
	}
}

// TestMarshalYAMLShape checks YAML uses OpenAPI's snake/lowercase keys rather
// than Go field names.
func TestMarshalYAMLShape(t *testing.T) {
	doc := &Document{OpenAPI: "3.0.3", Info: Info{Title: "t", Version: "1.0.0"}, Paths: map[string]*PathItem{
		"/a": {Get: &Operation{Summary: "s", OperationID: "getA", Responses: map[string]*Response{"200": {Description: "ok"}}}},
	}}
	data, err := Marshal(doc, "yaml")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var generic map[string]any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if generic["openapi"] != "3.0.3" {
		t.Fatalf("yaml = %s", data)
	}
	if !strings.Contains(string(data), "operationId: getA") {
		t.Fatalf("yaml keys are not OpenAPI-style:\n%s", data)
	}
}

// TestBuildRefusesDuplicateMethodPath checks two requests that map to the same
// OpenAPI operation make the export fail instead of silently dropping one.
func TestBuildRefusesDuplicateMethodPath(t *testing.T) {
	p := model.Project{Name: "P", Source: "native", Calls: []model.Call{
		{Title: "A", Method: "GET", Endpoint: "http://a.test/x"},
		{Title: "B", Method: "GET", Endpoint: "http://b.test/x"},
	}}
	if _, err := Build(p); err == nil {
		t.Fatal("Build accepted two GET /x requests; one would be silently dropped")
	}
}

// TestParseMultipartBinaryBecomesFileField checks format: binary imports as a
// file field, matching what export writes for a file part.
func TestParseMultipartBinaryBecomesFileField(t *testing.T) {
	const doc = `openapi: 3.0.1
info: {title: P, version: "1"}
paths:
  /u:
    post:
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              properties:
                file: {type: string, format: binary}
                note: {type: string}
      responses:
        "200": {description: ok}
`
	p, err := Parse([]byte(doc), "x.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields := p.Calls[0].BodyFields
	if len(fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(fields))
	}
	byName := map[string]model.BodyField{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if byName["file"].Type != "file" {
		t.Fatalf("file field type = %q, want file", byName["file"].Type)
	}
	if byName["note"].Type != "" {
		t.Fatalf("note field type = %q, want text", byName["note"].Type)
	}
}
