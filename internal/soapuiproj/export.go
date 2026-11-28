package soapuiproj

import (
	"encoding/xml"
	"fmt"
	"strings"

	"soapgo/internal/model"
)

// The exporter builds a SoapUI-compatible document with encoding/xml, so the
// escaping and structure come from the standard library rather than hand-written
// strings. Element names keep the "con:" prefix SoapUI writes; the prefix is
// declared on the root with a literal xmlns:con attribute.

// xmlEmpty is an empty element such as <con:settings/>.
type xmlEmpty struct{}

type xmlExportProject struct {
	XMLName       xml.Name             `xml:"con:soapui-project"`
	XmlnsCon      string               `xml:"xmlns:con,attr"`
	ID            string               `xml:"id,attr,omitempty"`
	Name          string               `xml:"name,attr"`
	SoapUIVersion string               `xml:"soapui-version,attr"`
	Settings      xmlEmpty             `xml:"con:settings"`
	Interfaces    []xmlExportInterface `xml:"con:interface"`
	TestSuites    []xmlExportTestSuite `xml:"con:testSuite"`
	Properties    xmlEmpty             `xml:"con:properties"`
}

type xmlExportInterface struct {
	XMLName     xml.Name             `xml:"con:interface"`
	XmlnsXsi    string               `xml:"xmlns:xsi,attr"`
	XsiType     string               `xml:"xsi:type,attr"`
	Name        string               `xml:"name,attr"`
	Type        string               `xml:"type,attr"`
	SoapVersion string               `xml:"soapVersion,attr"`
	DefCache    xmlExportDefCache    `xml:"con:definitionCache"`
	Endpoints   xmlExportEndpoints   `xml:"con:endpoints"`
	Operations  []xmlExportOperation `xml:"con:operation"`
}

type xmlExportDefCache struct {
	XMLName  xml.Name `xml:"con:definitionCache"`
	Type     string   `xml:"type,attr"`
	RootPart string   `xml:"rootPart,attr,omitempty"`
}

type xmlExportEndpoints struct {
	XMLName   xml.Name `xml:"con:endpoints"`
	Endpoints []string `xml:"con:endpoint"`
}

type xmlExportOperation struct {
	XMLName  xml.Name        `xml:"con:operation"`
	Name     string          `xml:"name,attr"`
	Action   string          `xml:"action,attr"`
	Type     string          `xml:"type,attr"`
	Settings xmlEmpty        `xml:"con:settings"`
	Calls    []xmlExportCall `xml:"con:call"`
}

type xmlExportCall struct {
	XMLName   xml.Name      `xml:"con:call"`
	ID        string        `xml:"id,attr"`
	Name      string        `xml:"name,attr"`
	Settings  xmlEmpty      `xml:"con:settings"`
	Endpoint  string        `xml:"con:endpoint,omitempty"`
	Request   xmlExportBody `xml:"con:request"`
	WsaConfig *xmlExportWsa `xml:"con:wsaConfig,omitempty"`
}

type xmlExportWsa struct {
	Action string `xml:"action,attr"`
}

// xmlExportBody is the request payload: CDATA when the body has no "]]>", else
// escaped character data (a "]]>" cannot appear inside a CDATA section).
type xmlExportBody struct {
	CDATA string `xml:",cdata"`
	Text  string `xml:",chardata"`
}

type xmlExportTestSuite struct {
	XMLName   xml.Name            `xml:"con:testSuite"`
	ID        string              `xml:"id,attr"`
	Name      string              `xml:"name,attr"`
	TestCases []xmlExportTestCase `xml:"con:testCase"`
}

type xmlExportTestCase struct {
	Name  string              `xml:"name,attr"`
	Steps []xmlExportTestStep `xml:"con:testStep"`
}

type xmlExportTestStep struct {
	Type   string          `xml:"type,attr"`
	Name   string          `xml:"name,attr"`
	Config xmlExportConfig `xml:"con:config"`
}

type xmlExportConfig struct {
	Request xmlExportConfigReq `xml:"con:request"`
}

type xmlExportConfigReq struct {
	Action   string `xml:"action,attr,omitempty"`
	Endpoint string `xml:"con:endpoint,omitempty"`
	CDATA    string `xml:",cdata"`
	Text     string `xml:",chardata"`
}

// Marshal serialises a model.Project back into a SoapUI-compatible
// soapui-project XML document. The output is designed to round-trip through
// Parse (and to be openable by SoapUI itself for the parts we model:
// interface / operation / call / request, plus test suites).
func Marshal(p model.Project) ([]byte, error) {
	// A native (plain HTTP) project has no SoapUI representation: refuse it
	// rather than emit a file that silently drops its group/call tree.
	if !p.CanExportSoapUI() {
		return nil, fmt.Errorf("普通 HTTP 项目不支持导出 SoapUI XML")
	}
	out := xmlExportProject{
		XmlnsCon:      "http://eviware.com/soapui/config",
		ID:            projectID(p),
		Name:          nameOr(p.Name, "soap-web-project"),
		SoapUIVersion: "5.7.0",
	}
	for _, svc := range p.Services {
		out.Interfaces = append(out.Interfaces, exportInterface(svc))
	}
	for _, ts := range p.TestSuites {
		out.TestSuites = append(out.TestSuites, exportTestSuite(ts))
	}
	body, err := xml.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func exportInterface(svc model.Service) xmlExportInterface {
	xi := xmlExportInterface{
		XmlnsXsi:    "http://www.w3.org/2001/XMLSchema-instance",
		XsiType:     "con:WsdlInterface",
		Name:        svc.Name,
		Type:        "wsdl",
		SoapVersion: "1_1",
		DefCache:    xmlExportDefCache{Type: "TEXT", RootPart: svc.Wsdl},
	}
	seenEP := map[string]bool{}
	addEP := func(ep string) {
		ep = strings.TrimSpace(ep)
		if ep == "" || seenEP[ep] {
			return
		}
		seenEP[ep] = true
		xi.Endpoints.Endpoints = append(xi.Endpoints.Endpoints, ep)
	}
	for _, op := range svc.Operations {
		addEP(op.Endpoint)
		for _, c := range op.Calls {
			addEP(c.Endpoint)
		}
	}
	for _, op := range svc.Operations {
		xo := xmlExportOperation{Name: op.Name, Action: op.SoapAction, Type: "Request-Response"}
		for _, c := range op.Calls {
			xo.Calls = append(xo.Calls, exportCall(op, c))
		}
		xi.Operations = append(xi.Operations, xo)
	}
	return xi
}

func exportCall(op model.Operation, c model.Call) xmlExportCall {
	title := c.Title
	if title == "" {
		title = op.Name
	}
	xc := xmlExportCall{
		ID:       callID(c),
		Name:     title,
		Endpoint: firstNonEmpty(c.Endpoint, op.Endpoint),
		Request:  exportBody(firstNonEmpty(c.Request, op.Request)),
	}
	if action := firstNonEmpty(c.Action, op.SoapAction); action != "" {
		xc.WsaConfig = &xmlExportWsa{Action: action}
	}
	return xc
}

func exportTestSuite(ts model.TestSuite) xmlExportTestSuite {
	x := xmlExportTestSuite{ID: newIDIfEmpty(ts.Name), Name: ts.Name}
	for _, tc := range ts.TestCases {
		xtc := xmlExportTestCase{Name: tc.Name}
		for _, st := range tc.Steps {
			step := xmlExportTestStep{Type: st.Type, Name: st.Name}
			step.Config.Request.Endpoint = st.Endpoint
			step.Config.Request.Action = st.Action
			step.Config.Request.CDATA, step.Config.Request.Text = bodyParts(st.Request)
			xtc.Steps = append(xtc.Steps, step)
		}
		x.TestCases = append(x.TestCases, xtc)
	}
	return x
}

// exportBody wraps a request body for a con:request element.
func exportBody(body string) xmlExportBody {
	cdata, text := bodyParts(body)
	return xmlExportBody{CDATA: cdata, Text: text}
}

// bodyParts returns the CDATA and escaped-text parts of a body: a body with no
// "]]>" goes in CDATA, one that has it goes in escaped text so the XML stays
// valid.
func bodyParts(body string) (cdata, text string) {
	if strings.Contains(body, "]]>") {
		return "", body
	}
	return body, ""
}

func nameOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func projectID(p model.Project) string {
	if p.ID != "" {
		return p.ID
	}
	return "soap-web-project-id"
}

func callID(c model.Call) string {
	if c.ID != "" {
		return c.ID
	}
	return "call-" + strings.ReplaceAll(c.Title, " ", "-")
}

func newIDIfEmpty(seed string) string {
	return "suite-" + strings.ReplaceAll(seed, " ", "-")
}
