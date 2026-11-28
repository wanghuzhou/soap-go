// Package textfmt holds pure JSON and XML formatting helpers. They used to live
// in main; moving them here keeps app.go focused on UI state and operations and
// makes the formatters reusable and independently testable.
package textfmt

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
)

// FormatJSON pretty-prints a JSON document with two-space indentation. An empty
// input is returned unchanged.
func FormatJSON(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s, nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// MinifyJSON removes insignificant whitespace from a JSON document.
func MinifyJSON(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s, nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PrettyJSON pretty-prints a JSON body, leaving it unchanged when it is not
// valid JSON.
func PrettyJSON(s string) string {
	out, err := FormatJSON(s)
	if err != nil {
		return s
	}
	return out
}

// FormatXML re-indents an XML document while keeping every tag, namespace
// prefix, attribute and CDATA section as written. An element whose only content
// is text stays on one line (`<a>test</a>`), and an empty `<a></a>` keeps that
// form; a source `<b/>` stays self-closing.
func FormatXML(s string) (string, error) {
	toks, err := xmlTokenize(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	if len(toks) == 0 {
		return s, nil
	}
	var b strings.Builder
	depth := 0
	indent := func() { b.WriteString(strings.Repeat("  ", depth)) }
	for i := 0; i < len(toks); i++ {
		tk := toks[i]
		raw := strings.TrimSpace(tk.raw)
		if tk.kind == 's' {
			// Look ahead past structural whitespace: an empty element or an
			// element with only inline content (text/CDATA/comment) is one line.
			j := i + 1
			var inline strings.Builder
			for j < len(toks) {
				n := toks[j]
				if n.kind == 't' && strings.TrimSpace(n.raw) == "" {
					j++
					continue
				}
				if isInlineXMLToken(n) {
					inline.WriteString(n.raw)
					j++
					continue
				}
				break
			}
			if j < len(toks) && toks[j].kind == 'e' {
				indent()
				b.WriteString(raw)
				b.WriteString(inline.String())
				b.WriteString(strings.TrimSpace(toks[j].raw))
				b.WriteString("\n")
				i = j
				continue
			}
			indent()
			b.WriteString(raw)
			b.WriteString("\n")
			depth++
			continue
		}
		if raw == "" {
			continue
		}
		if tk.kind == 'e' {
			depth--
			if depth < 0 {
				depth = 0
			}
		}
		indent()
		b.WriteString(raw)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// MinifyXML removes structural whitespace from an XML document, keeping text
// values' own spaces.
func MinifyXML(s string) (string, error) {
	toks, err := xmlTokenize(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, tk := range toks {
		if tk.kind == 't' {
			if strings.TrimSpace(tk.raw) == "" {
				continue // structural whitespace only
			}
			b.WriteString(tk.raw) // keep the text value's own spaces
			continue
		}
		b.WriteString(strings.TrimSpace(tk.raw))
	}
	return b.String(), nil
}

// PrettyXML formats an XML body for display, leaving malformed bodies as-is.
func PrettyXML(s string) string {
	out, err := FormatXML(s)
	if err != nil {
		return s
	}
	return out
}

// xmlTok is one lexical token of an XML document, with its original bytes.
type xmlTok struct {
	kind byte   // 's' start, 'e' end, 'x' self-closing, 't' text, 'q' comment/CDATA/PI/directive
	raw  string // the token exactly as written
}

// xmlTokenize splits a document into tokens with encoding/xml's tokenizer, but
// keeps each token's original bytes (via Decoder.InputOffset), so namespace
// prefixes, attribute quoting, entity references and CDATA sections survive.
// A regex cannot tokenize XML correctly: a ">" may sit inside an attribute
// value, and "<" inside CDATA or a comment.
func xmlTokenize(s string) ([]xmlTok, error) {
	s = strings.TrimPrefix(s, "\ufeff")
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = false
	var toks []xmlTok
	prev := int64(0)
	for {
		t, err := dec.RawToken()
		if err == io.EOF {
			return toks, nil
		}
		if err != nil {
			return nil, err
		}
		end := dec.InputOffset()
		raw := s[prev:end]
		prev = end
		switch t.(type) {
		case xml.StartElement:
			if strings.HasSuffix(strings.TrimSpace(raw), "/>") {
				toks = append(toks, xmlTok{'x', raw})
			} else {
				toks = append(toks, xmlTok{'s', raw})
			}
		case xml.EndElement:
			if strings.TrimSpace(raw) == "" {
				continue // synthetic end tag of a self-closing element
			}
			toks = append(toks, xmlTok{'e', raw})
		case xml.CharData:
			if strings.HasPrefix(strings.TrimSpace(raw), "<![CDATA[") {
				toks = append(toks, xmlTok{'q', raw})
			} else {
				toks = append(toks, xmlTok{'t', raw})
			}
		default: // xml.Comment, xml.ProcInst, xml.Directive
			toks = append(toks, xmlTok{'q', raw})
		}
	}
}

// isInlineXMLToken reports whether a token is content that may sit inside an
// otherwise empty element on one line: text, a CDATA section or a comment.
func isInlineXMLToken(tk xmlTok) bool {
	switch tk.kind {
	case 't':
		return strings.TrimSpace(tk.raw) != ""
	case 'q':
		r := strings.TrimSpace(tk.raw)
		return strings.HasPrefix(r, "<![CDATA[") || strings.HasPrefix(r, "<!--")
	}
	return false
}
