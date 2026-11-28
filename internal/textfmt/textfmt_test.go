package textfmt

import "testing"

func TestFormatJSON(t *testing.T) {
	out, err := FormatJSON(`{"a":1,"b":[2,3]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}" {
		t.Fatalf("FormatJSON = %q", out)
	}
}

func TestMinifyJSON(t *testing.T) {
	out, err := MinifyJSON("{\n  \"a\": 1\n}")
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"a":1}` {
		t.Fatalf("MinifyJSON = %q", out)
	}
}

func TestMinifyXML(t *testing.T) {
	out, err := MinifyXML("<a>\n  <b>1</b>\n</a>")
	if err != nil {
		t.Fatal(err)
	}
	if out != "<a><b>1</b></a>" {
		t.Fatalf("MinifyXML = %q", out)
	}
}

func TestPrettyFallbacks(t *testing.T) {
	if got := PrettyJSON("not json"); got != "not json" {
		t.Fatalf("PrettyJSON invalid = %q", got)
	}
	if got := PrettyXML("<a><b>1</b></a>"); got != "<a>\n  <b>1</b>\n</a>" {
		t.Fatalf("PrettyXML = %q", got)
	}
}
