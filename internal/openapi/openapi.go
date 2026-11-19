// Package openapi reads and writes OpenAPI 3.x descriptions for the plain-HTTP
// (native) projects of this app.
//
// It is deliberately a narrow mapping, not a general-purpose OpenAPI library:
// only the parts a native project can represent are handled — paths and their
// operations, path/query/header parameters, request bodies and response
// examples. Unknown keywords are ignored on read and omitted on write, so a
// document survives a round trip with its structure intact even when it uses
// features (callbacks, links, security schemes...) this tool does not model.
package openapi

// Document is the subset of an OpenAPI 3.x document this package understands.
type Document struct {
	OpenAPI    string               `json:"openapi" yaml:"openapi"`
	Info       Info                 `json:"info" yaml:"info"`
	Servers    []Server             `json:"servers,omitempty" yaml:"servers,omitempty"`
	Tags       []Tag                `json:"tags,omitempty" yaml:"tags,omitempty"`
	Paths      map[string]*PathItem `json:"paths" yaml:"paths"`
	Components *Components          `json:"components,omitempty" yaml:"components,omitempty"`
}

// Info is the document's title block.
type Info struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string `json:"version" yaml:"version"`
}

// Server is one base URL the API is served from.
type Server struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Tag groups operations, shown as a folder on import.
type Tag struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// PathItem holds the operations available at one path. A path-level Parameters
// list applies to every operation below it.
type PathItem struct {
	Ref        string      `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Get        *Operation  `json:"get,omitempty" yaml:"get,omitempty"`
	Put        *Operation  `json:"put,omitempty" yaml:"put,omitempty"`
	Post       *Operation  `json:"post,omitempty" yaml:"post,omitempty"`
	Delete     *Operation  `json:"delete,omitempty" yaml:"delete,omitempty"`
	Options    *Operation  `json:"options,omitempty" yaml:"options,omitempty"`
	Head       *Operation  `json:"head,omitempty" yaml:"head,omitempty"`
	Patch      *Operation  `json:"patch,omitempty" yaml:"patch,omitempty"`
	Trace      *Operation  `json:"trace,omitempty" yaml:"trace,omitempty"`
	Parameters []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// methodOperation pairs an HTTP method with its operation for iteration.
type methodOperation struct {
	method string
	op     *Operation
}

// operations returns the operations of a path item in a stable method order.
func (p *PathItem) operations() []methodOperation {
	if p == nil {
		return nil
	}
	pairs := []methodOperation{
		{"get", p.Get},
		{"put", p.Put},
		{"post", p.Post},
		{"delete", p.Delete},
		{"options", p.Options},
		{"head", p.Head},
		{"patch", p.Patch},
		{"trace", p.Trace},
	}
	out := make([]methodOperation, 0, len(pairs))
	for _, pair := range pairs {
		if pair.op != nil {
			out = append(out, pair)
		}
	}
	return out
}

// setMethod stores op under the given HTTP method, ignoring unknown methods.
func (p *PathItem) setMethod(method string, op *Operation) {
	switch method {
	case "GET":
		p.Get = op
	case "PUT":
		p.Put = op
	case "POST":
		p.Post = op
	case "DELETE":
		p.Delete = op
	case "OPTIONS":
		p.Options = op
	case "HEAD":
		p.Head = op
	case "PATCH":
		p.Patch = op
	case "TRACE":
		p.Trace = op
	}
}

// Operation is one HTTP method at one path.
type Operation struct {
	Tags        []string             `json:"tags,omitempty" yaml:"tags,omitempty"`
	Summary     string               `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	OperationID string               `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Deprecated  bool                 `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Parameters  []Parameter          `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses,omitempty" yaml:"responses,omitempty"`
}

// Parameter is a path, query or header parameter.
type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"`
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool    `json:"required,omitempty" yaml:"required,omitempty"`
	Deprecated  bool    `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Example     any     `json:"example,omitempty" yaml:"example,omitempty"`
	Schema      *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RequestBody is the payload an operation accepts.
type RequestBody struct {
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool                  `json:"required,omitempty" yaml:"required,omitempty"`
	Content     map[string]*MediaType `json:"content" yaml:"content"`
}

// MediaType ties a schema and an example to one media type.
type MediaType struct {
	Schema  *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
	Example any     `json:"example,omitempty" yaml:"example,omitempty"`
}

// Response is one possible reply, keyed by status code.
type Response struct {
	Description string                `json:"description" yaml:"description"`
	Content     map[string]*MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// Schema is the subset of JSON Schema used to describe bodies and parameters.
type Schema struct {
	Ref         string             `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Example     any                `json:"example,omitempty" yaml:"example,omitempty"`
	Default     any                `json:"default,omitempty" yaml:"default,omitempty"`
	Enum        []any              `json:"enum,omitempty" yaml:"enum,omitempty"`
}

// Components holds the reusable schemas a document may reference.
type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty" yaml:"schemas,omitempty"`
}
