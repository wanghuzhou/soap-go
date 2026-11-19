// Package model defines the shared data structures used across the application.
package model

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Project is the root container. It mirrors the structure of a SoapUI project
// closely enough for import, while staying simple for our own needs.
//
// A native (plain HTTP) project keeps its own tree in Groups and Calls: Groups
// is a flat list linked by ParentID (like a filesystem), and Calls holds every
// request directly, with FolderID naming the group it sits in ("" = the project
// root). A WSDL or SoapUI import keeps using Services/Operations instead. The
// two shapes can coexist in one project: an imported SOAP service lives under
// Services while the user's own HTTP requests live under Groups/Calls.
type Project struct {
	ID     string         `json:"id"` // stable identifier for CRUD
	Name   string         `json:"name"`
	Source string         `json:"source"` // e.g. "soapui", "wsdl", "wsdl-url" or "native"
	Proxy  *ProxySettings `json:"proxy,omitempty"`
	// Headers are sent with every request of this project. A call may override
	// an individual value; see Call.Headers.
	Headers []Header `json:"headers,omitempty"`
	// Groups is the flat folder tree of a native project. ParentID is "" for a
	// group directly under the project. IDs are unique across the project.
	Groups []Group `json:"groups,omitempty"`
	// Calls are requests placed directly under the project or one of its
	// Groups. Calls that belong to an imported SOAP operation live in
	// Service.Operations[].Calls instead.
	Calls      []Call      `json:"calls,omitempty"`
	Services   []Service   `json:"services"`
	TestSuites []TestSuite `json:"testSuites"`
}

// Group is a folder of the native request tree. ParentID links it to another
// group; "" means it sits directly under the project. The flat layout (rather
// than nested slices) keeps lookups, deletes and id assignment simple.
type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
}

// CanExportOpenAPI reports whether this project may be exported as an OpenAPI
// 3.x description. Only a native (plain HTTP) project maps onto OpenAPI; SOAP
// services imported from SoapUI or a WSDL have no matching shape.
func (p Project) CanExportOpenAPI() bool {
	return p.Source == "native"
}

// CanExportSoapUI reports whether this project may be exported as SoapUI XML.
//
// Only projects that came from SoapUI or a WSDL describe SoapUI's own data
// model. A native (plain HTTP) project has no SoapUI representation, so the UI
// hides the export action and the export entry point refuses it. Keeping the
// decision here means the UI and the exporter cannot disagree.
func (p Project) CanExportSoapUI() bool {
	switch p.Source {
	case "soapui", "wsdl", "wsdl-url":
		return true
	}
	return false
}

// Header is one HTTP header on a project or a call. A slice rather than a map
// keeps the user's order and lets an empty Value mean "send the header with no
// value" instead of being indistinguishable from "not set".
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ValidateHeaders rejects names that cannot be sent: a request with a bad
// header name fails at the transport layer with an opaque error, so it is
// caught here where the message can name the culprit.
func ValidateHeaders(headers []Header) error {
	for _, h := range headers {
		name := strings.TrimSpace(h.Name)
		if name == "" {
			return errors.New("请求头名称不能为空")
		}
		for _, r := range name {
			if r <= ' ' || r >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", r) {
				return fmt.Errorf("请求头名称 %q 含非法字符", name)
			}
		}
		if strings.ContainsAny(h.Value, "\r\n") {
			return fmt.Errorf("请求头 %q 的值不能包含换行", name)
		}
	}
	return nil
}

// AuthSettings is one request's authentication. Type is "basic", "bearer" or
// (the zero value) no authentication. Enabled turns the configuration off
// without forgetting the credentials; Prefix is the Bearer scheme word and
// defaults to "Bearer" when empty.
type AuthSettings struct {
	Type     string `json:"type,omitempty"`
	Enabled  bool   `json:"enabled,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
}

// Authorization returns the Authorization header value this configuration
// produces, or "" when it is absent, disabled or missing its credentials.
func (a *AuthSettings) Authorization() string {
	if a == nil || !a.Enabled {
		return ""
	}
	switch a.Type {
	case "basic":
		if a.Username == "" && a.Password == "" {
			return ""
		}
		raw := a.Username + ":" + a.Password
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	case "bearer":
		if a.Token == "" {
			return ""
		}
		prefix := strings.TrimSpace(a.Prefix)
		if prefix == "" {
			prefix = "Bearer"
		}
		return prefix + " " + a.Token
	}
	return ""
}

// Clone returns a copy that shares nothing mutable with a.
func (a *AuthSettings) Clone() *AuthSettings {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

// ProxySettings controls how requests of a project reach the network.
// Mode is one of "system" (use OS/environment proxy), "direct" (no proxy) or
// "custom". For custom, Type is "http" or "socks5"; Username/Password may be
// empty when the proxy needs no authentication.
type ProxySettings struct {
	Mode     string `json:"mode"` // system | direct | custom
	Type     string `json:"type"` // http | socks5
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate reports whether the settings are usable.
func (p *ProxySettings) Validate() error {
	if p == nil {
		return nil
	}
	switch p.Mode {
	case "", "system", "direct":
		return nil
	case "custom":
		if p.Host == "" {
			return errors.New("自定义代理缺少主机地址")
		}
		if p.Port <= 0 || p.Port > 65535 {
			return errors.New("自定义代理端口无效")
		}
		if p.Type != "http" && p.Type != "socks5" {
			return errors.New("自定义代理类型必须是 http 或 socks5")
		}
		return nil
	default:
		return errors.New("未知的代理模式")
	}
}

// Service represents a WSDL-described interface (a SoapUI "interface").
type Service struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Wsdl       string      `json:"wsdl"` // local path or URL
	Operations []Operation `json:"operations"`
}

// Operation is a single callable operation derived from the WSDL.
//
// An operation may carry several saved request samples (SoapUI lets you keep
// multiple named requests per operation, each with its own parameters). These
// live in Calls. Request mirrors the first call for convenience.
type Operation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SoapAction string `json:"soapAction"`
	Endpoint   string `json:"endpoint"`
	Style      string `json:"style"`     // document / rpc
	Use        string `json:"use"`       // literal / encoded
	InputName  string `json:"inputName"` // wrapper element name, if known
	Request    string `json:"request"`   // generated or first saved XML template
	Template   string `json:"template"`  // pure structural skeleton (no business data), from WSDL
	Calls      []Call `json:"calls"`     // saved request samples (with titles)

	// Bindings lists every WSDL binding that exposes this operation (for
	// example "SvcSoap11" and "SvcSoap12"). A WSDL often declares the same
	// operation twice, once per SOAP version; the parser keeps one entry per
	// operation and records the alternatives here so the UI can show which
	// protocols are available instead of listing duplicate rows.
	Bindings []string `json:"bindings,omitempty"`
}

// Call is a named saved request sample within an operation (a SoapUI <con:call>).
type Call struct {
	ID       string `json:"id"`
	Title    string `json:"title"` // e.g. "5.6.5 物价查询(药品等)"
	Endpoint string `json:"endpoint"`
	Action   string `json:"action"`
	Request  string `json:"request"` // raw XML body
	// Kind is "soap" or "http". Empty means "infer from where the call sits":
	// a call under a Service.Operation is SOAP, one under Project/Group is HTTP.
	Kind string `json:"kind,omitempty"`
	// Method is the HTTP method. Empty means POST for SOAP, and POST for an
	// inference-less HTTP call; see EffectiveMethod.
	Method string `json:"method,omitempty"`
	// ContentType is the request body's media type. Empty means the kind's
	// default (see DefaultContentType).
	ContentType string `json:"contentType,omitempty"`
	// BodyType is the shape of a plain HTTP body: none, json, xml, graphql,
	// text, urlencoded, multipart or binary. Empty means "infer from
	// ContentType/Request"; see NormalizeBodyType.
	BodyType string `json:"bodyType,omitempty"`
	// BodyFields holds the name/value pairs of a urlencoded or multipart body.
	BodyFields []BodyField `json:"bodyFields,omitempty"`
	// BodyFile is the source path of a binary body.
	BodyFile string `json:"bodyFile,omitempty"`
	// FolderID names the Group this native request sits in; "" is the project
	// root. Unused by an operation's SOAP samples.
	FolderID string `json:"folderId,omitempty"`
	// Description is free-form notes about the request, such as the operation
	// description carried over from an OpenAPI import.
	Description string `json:"description,omitempty"`
	// Headers override the project's defaults for this sample only.
	Headers []Header `json:"headers,omitempty"`
	// Auth is this sample's authentication; nil means none.
	Auth *AuthSettings `json:"auth,omitempty"`
	// TimeoutMS bounds this sample's request. 0 means "use the default".
	TimeoutMS int `json:"timeoutMs,omitempty"`
	// Response is the last response the user saved for this sample. It is
	// persisted with the workspace so reopening the sample shows it without
	// sending the request again.
	Response *SavedResponse `json:"response,omitempty"`
}

// Request kinds.
const (
	KindSOAP = "soap"
	KindHTTP = "http"
)

// HTTPMethods are the methods a plain HTTP request may use, in menu order.
var HTTPMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// KindOrDefault returns the call's kind, falling back to fallback when unset.
func (c Call) KindOrDefault(fallback string) string {
	if c.Kind == KindSOAP || c.Kind == KindHTTP {
		return c.Kind
	}
	return fallback
}

// EffectiveMethod is the method a request is actually sent with: SOAP is always
// POST; an HTTP call uses its own method, defaulting to POST when unset.
func EffectiveMethod(kind, method string) string {
	if kind == KindSOAP {
		return "POST"
	}
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		return "POST"
	}
	return m
}

// MethodHasBody reports whether method normally carries a request body.
func MethodHasBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return false
	}
	return true
}

// DefaultContentType is the body media type for a kind when none is set.
func DefaultContentType(kind string) string {
	if kind == KindSOAP {
		return "text/xml; charset=utf-8"
	}
	return "application/json"
}

// BodyField is one name/value pair of a urlencoded or multipart body.
type BodyField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Plain-HTTP body shapes, mirroring the body-type menu.
const (
	BodyNone       = "none"
	BodyJSON       = "json"
	BodyXML        = "xml"
	BodyGraphQL    = "graphql"
	BodyText       = "text"
	BodyURLEncoded = "urlencoded"
	BodyMultipart  = "multipart"
	BodyBinary     = "binary"
)

// NormalizeBodyType maps an arbitrary value to a known body type, defaulting to
// "" only when the value is unknown.
func NormalizeBodyType(s string) string {
	switch s {
	case BodyNone, BodyJSON, BodyXML, BodyGraphQL, BodyText,
		BodyURLEncoded, BodyMultipart, BodyBinary:
		return s
	}
	return ""
}

// InferBodyType derives a body type for a legacy call that has none stored,
// from its content type and body. Without a telling content type, a body
// written as JSON or XML is classified by its first character.
func InferBodyType(contentType, body string) string {
	ct := strings.ToLower(contentType)
	trimmed := strings.TrimSpace(body)
	switch {
	case strings.Contains(ct, "json"):
		return BodyJSON
	case strings.Contains(ct, "xml"):
		return BodyXML
	case strings.Contains(ct, "x-www-form-urlencoded"):
		return BodyURLEncoded
	case strings.Contains(ct, "multipart/form-data"):
		return BodyMultipart
	case trimmed == "":
		return BodyNone
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		return BodyJSON
	case strings.HasPrefix(trimmed, "<"):
		return BodyXML
	default:
		return BodyText
	}
}

// BodyTypeContentType is the media type a text body type implies.
func BodyTypeContentType(bt string) string {
	switch bt {
	case BodyJSON, BodyGraphQL:
		return "application/json"
	case BodyXML:
		return "application/xml"
	case BodyText:
		return "text/plain"
	case BodyURLEncoded:
		return "application/x-www-form-urlencoded"
	case BodyMultipart:
		return "multipart/form-data"
	case BodyBinary:
		return "application/octet-stream"
	}
	return ""
}

// BodyTypeHasTextEditor reports whether a body type is edited as free text.
func BodyTypeHasTextEditor(bt string) bool {
	switch bt {
	case BodyJSON, BodyXML, BodyGraphQL, BodyText:
		return true
	}
	return false
}

// BodyTypeHasFields reports whether a body type is edited as name/value pairs.
func BodyTypeHasFields(bt string) bool {
	return bt == BodyURLEncoded || bt == BodyMultipart
}

// SavedResponse is a response kept with a request sample. It deliberately
// mirrors the useful parts of SendResponse but omits transient fields.
type SavedResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMS int64             `json:"durationMs"`
	Size       int               `json:"size"`
	Truncated  bool              `json:"truncated,omitempty"`
}

// MaxSavedResponseBytes caps a persisted response. It matches the transport
// read cap, so anything the UI could display can also be saved.
const MaxSavedResponseBytes = 16 << 20

// ValidateSavedResponse rejects a response body larger than the cap.
func ValidateSavedResponse(r *SavedResponse) error {
	if r == nil {
		return nil
	}
	if len(r.Body) > MaxSavedResponseBytes {
		return fmt.Errorf("响应内容超过 %d MB 上限", MaxSavedResponseBytes>>20)
	}
	return nil
}

// ValidateProjectResponses checks every saved response in a project, so a
// hand-written workspace JSON cannot bypass the size limit.
func ValidateProjectResponses(p Project) error {
	for _, svc := range p.Services {
		for _, op := range svc.Operations {
			for _, c := range op.Calls {
				if err := ValidateSavedResponse(c.Response); err != nil {
					return fmt.Errorf("请求样例「%s」: %w", c.Title, err)
				}
			}
		}
	}
	return nil
}

// DefaultTimeoutMS applies when neither the sample nor the caller sets one.
// Slow legacy services sometimes need more; a runaway request must not hold a
// connection open forever.
const DefaultTimeoutMS = 60000

// MinTimeoutMS / MaxTimeoutMS bound the value a caller may ask for. The lower
// bound keeps a typo ("1") from turning every request into an instant timeout.
const (
	MinTimeoutMS = 1000
	MaxTimeoutMS = 600000
)

// ClampTimeout returns ms bounded to [MinTimeoutMS, MaxTimeoutMS], falling back
// to DefaultTimeoutMS when unset. The API layer and transport share this so the
// bounds cannot drift apart.
func ClampTimeout(ms int) int {
	if ms <= 0 {
		return DefaultTimeoutMS
	}
	if ms < MinTimeoutMS {
		return MinTimeoutMS
	}
	if ms > MaxTimeoutMS {
		return MaxTimeoutMS
	}
	return ms
}

// DefaultRequestBody returns the body a new request sample should start from:
// the operation's structural template, then its generated request, then the
// first saved call. Keeping the order here means seeding a call behaves the
// same everywhere instead of each caller re-implementing the fallback chain.
func (op *Operation) DefaultRequestBody() string {
	if op.Template != "" {
		return op.Template
	}
	if op.Request != "" {
		return op.Request
	}
	if len(op.Calls) > 0 {
		return op.Calls[0].Request
	}
	return ""
}

// SeedCall builds an operation's default request sample from its structural
// template. It reports false when no body can be derived, so callers can skip
// operations that would otherwise save an empty call.
func SeedCall(op *Operation) (Call, bool) {
	body := op.DefaultRequestBody()
	if body == "" {
		return Call{}, false
	}
	title := op.Name
	if title == "" {
		title = "请求"
	}
	return Call{
		Title:    title,
		Endpoint: op.Endpoint,
		Action:   op.SoapAction,
		Request:  body,
	}, true
}

// TestSuite / TestCase / TestStep mirror SoapUI's hierarchy. A TestStep that
// carries a request can be replayed directly without WSDL parsing.
//
// These types are pass-through only: they are read from and written back to
// SoapUI projects so no data is lost, but the UI neither displays nor executes
// them.
type TestSuite struct {
	Name      string     `json:"name"`
	TestCases []TestCase `json:"testCases"`
}

type TestCase struct {
	Name  string     `json:"name"`
	Steps []TestStep `json:"steps"`
}

type TestStep struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // request / groovy / properties / unknown
	Endpoint string `json:"endpoint"`
	Action   string `json:"action"`
	Request  string `json:"request"` // raw XML body saved in the project
}

// ValidateEndpoint reports whether a request URL is one this tool may call: an
// absolute http or https URL with a host.
//
// Without this check the scheme was only rejected deep inside client.Do, which
// produced a confusing "unsupported protocol scheme" error after the request had
// already been built — and, more importantly, left /api/send accepting arbitrary
// schemes. The service listens on loopback by default, but -addr can expose it on
// a network, where an unauthenticated endpoint that fetches any URL is an
// unwanted request proxy. Remote WSDL fetching (wsdlfetch) already enforced
// exactly these rules; this is the same boundary for sending.
func ValidateEndpoint(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("Endpoint 地址无效: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("Endpoint 仅支持 http/https 协议，当前为 %q", u.Scheme)
	}
	if u.Host == "" || u.Hostname() == "" {
		return errors.New("Endpoint 缺少主机名")
	}
	return nil
}

// SendRequest is the payload the frontend posts to execute an HTTP/SOAP call.
type SendRequest struct {
	ProjectID   string            `json:"projectId"` // used to resolve proxy settings
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	ContentType string            `json:"contentType"`
	TimeoutMS   int               `json:"timeoutMs"`
}

// SendResponse is what we return to the frontend after a call.
type SendResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMS int64             `json:"durationMs"`
	Size       int               `json:"size"`
	Truncated  bool              `json:"truncated,omitempty"` // body hit the size cap
	Error      string            `json:"error,omitempty"`
}
