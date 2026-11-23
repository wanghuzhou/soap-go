package httpfile

import (
	"strings"
	"testing"

	"soapgo/internal/model"
)

// sample mirrors the shape of a real JetBrains .http file: a header comment
// block, variables, "###" separated requests, per-request comments, JSON bodies
// and a "> {% ... %}" response handler that must be ignored.
const sample = `# =============================================================================
# 安徽监管 - 诊疗科室信息  服务间 RPC
# 统一响应结构 ResultDTO：code=0 为成功
# =============================================================================

@host = http://127.0.0.1:8099
@hospitalId = 20011
# 主键
@id = 1

### 1. 诊疗科室信息 - 新增
# 路径：POST /rpc/ahSuperviseBase/orgDeptAdd
# 响应示例
# { "code": 0, "errMsg": "新增成功", "data": null }
POST {{host}}/rpc/ahSuperviseBase/orgDeptAdd
Content-Type: application/json
Accept: application/json

{
  "hospitalId": "{{hospitalId}}",
  "deptName": "互联网医院内科"
}

> {%
    client.test("业务成功（code=0）", function () {
        client.assert(response.body.code === 0, "业务失败");
    });
%}


### 2. 诊疗科室信息 - 删除
POST {{host}}/rpc/ahSuperviseBase/orgDeptDel
Content-Type: application/json

{
  "id": {{id}},
  "hospitalId": "{{hospitalId}}"
}
`

// TestParseSample checks the sample maps to two requests with substituted
// variables, split headers and a clean JSON body, and that the comments and the
// response handler are gone.
func TestParseSample(t *testing.T) {
	p, err := Parse([]byte(sample), "AHOrgDept-rpc.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "AHOrgDept-rpc" || p.Source != "native" {
		t.Fatalf("project = %q/%q", p.Name, p.Source)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(p.Calls))
	}

	add := p.Calls[0]
	if add.Title != "1. 诊疗科室信息 - 新增" {
		t.Errorf("title = %q", add.Title)
	}
	if add.Method != "POST" || add.Kind != model.KindHTTP {
		t.Errorf("method/kind = %q/%q", add.Method, add.Kind)
	}
	if want := "http://127.0.0.1:8099/rpc/ahSuperviseBase/orgDeptAdd"; add.Endpoint != want {
		t.Errorf("endpoint = %q, want %q", add.Endpoint, want)
	}
	if add.ContentType != "application/json" {
		t.Errorf("content type = %q", add.ContentType)
	}
	if len(add.Headers) != 1 || add.Headers[0].Name != "Accept" || add.Headers[0].Value != "application/json" {
		t.Errorf("headers = %+v (Content-Type must live on ContentType, not Headers)", add.Headers)
	}
	if add.BodyType != model.BodyJSON {
		t.Errorf("body type = %q", add.BodyType)
	}
	if !strings.Contains(add.Request, `"hospitalId": "20011"`) {
		t.Errorf("body was not substituted: %q", add.Request)
	}
	if strings.Contains(add.Request, "{{") || strings.Contains(add.Request, "client.test") {
		t.Errorf("body carries a placeholder or the response handler: %q", add.Request)
	}

	del := p.Calls[1]
	if want := "http://127.0.0.1:8099/rpc/ahSuperviseBase/orgDeptDel"; del.Endpoint != want {
		t.Errorf("delete endpoint = %q, want %q", del.Endpoint, want)
	}
	if !strings.Contains(del.Request, `"id": 1`) {
		t.Errorf("delete id not substituted: %q", del.Request)
	}
}

// TestParseUntitledUsesMethodAndPath checks a "###" block with no name gets a
// readable "METHOD /path" fallback.
func TestParseUntitledUsesMethodAndPath(t *testing.T) {
	p, err := Parse([]byte("@host = http://api.test\n\n###\nGET {{host}}/users/list\n"), "api.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Calls) != 1 {
		t.Fatalf("calls = %d", len(p.Calls))
	}
	if got := p.Calls[0].Title; got != "GET /users/list" {
		t.Errorf("title = %q", got)
	}
	if got := p.Calls[0].BodyType; got != model.BodyNone {
		t.Errorf("body type = %q, want none", got)
	}
}

// TestParseNameDirective checks "# @name foo" names the following request.
func TestParseNameDirective(t *testing.T) {
	src := "@base = http://api.test\n\n# @name listUsers\nGET {{base}}/users\n"
	p, err := Parse([]byte(src), "x.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Calls[0].Title; got != "listUsers" {
		t.Errorf("title = %q", got)
	}
}

// TestParseFormURLEncoded checks a form body becomes editable fields.
func TestParseFormURLEncoded(t *testing.T) {
	src := "@base = http://api.test\n\n### 登录\nPOST {{base}}/login\n" +
		"Content-Type: application/x-www-form-urlencoded\n\nuser=alice&pass=a%26b\n"
	p, err := Parse([]byte(src), "x.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	call := p.Calls[0]
	if call.BodyType != model.BodyURLEncoded {
		t.Fatalf("body type = %q", call.BodyType)
	}
	if len(call.BodyFields) != 2 || call.BodyFields[0].Name != "user" || call.BodyFields[0].Value != "alice" {
		t.Fatalf("fields = %+v", call.BodyFields)
	}
	if call.BodyFields[1].Value != "a&b" {
		t.Errorf("unescaped value = %q", call.BodyFields[1].Value)
	}
}

// TestParseInfersBodyFromContent checks a body without a Content-Type is still
// classified by its first character.
func TestParseInfersBodyFromContent(t *testing.T) {
	src := "@base = http://api.test\n\n### 推断\nPOST {{base}}/x\n\n{\"a\": 1}\n"
	p, err := Parse([]byte(src), "x.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Calls[0].BodyType; got != model.BodyJSON {
		t.Errorf("body type = %q", got)
	}
}

// TestParseRejectsCommentOnly checks a file with no request is an error rather
// than an empty project.
func TestParseRejectsCommentOnly(t *testing.T) {
	if _, err := Parse([]byte("# 只有注释\n# 没有请求\n"), "x.http"); err == nil {
		t.Fatal("Parse accepted a file with no request")
	}
}

// TestParseStripsBOM checks a BOM at the start does not break the first
// variable or request.
func TestParseStripsBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("@host = http://api.test\n\n### 一\nGET {{host}}/a\n")...)
	p, err := Parse(data, "x.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.Calls[0].Endpoint; got != "http://api.test/a" {
		t.Errorf("endpoint = %q", got)
	}
}

// TestParseDropsCommentsAroundBody checks comments before and after a body do
// not leak into it (a leaked note makes JSON invalid).
func TestParseDropsCommentsAroundBody(t *testing.T) {
	src := `### A
POST http://h/a
Content-Type: application/json

# 请求体示例
{"x": 1}

# 下一条请求的说明
### B
POST http://h/b
Content-Type: application/json

{"y": 2}
`
	p, err := Parse([]byte(src), "x.http")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("calls = %d", len(p.Calls))
	}
	if got := p.Calls[0].Request; got != `{"x": 1}` {
		t.Errorf("body A = %q, want the JSON only", got)
	}
	if p.Calls[0].BodyType != model.BodyJSON {
		t.Errorf("body A type = %q", p.Calls[0].BodyType)
	}
	if got := p.Calls[1].Request; got != `{"y": 2}` {
		t.Errorf("body B = %q", got)
	}
}
