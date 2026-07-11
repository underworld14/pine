package frontmatter

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Split ---------------------------------------------------------------

func TestSplitLF(t *testing.T) {
	raw := "---\nkey: value\n---\nbody content"
	fm, body, ok := Split(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if fm != "key: value\n" {
		t.Errorf("fm = %q, want %q", fm, "key: value\n")
	}
	if body != "body content" {
		t.Errorf("body = %q, want %q", body, "body content")
	}
}

func TestSplitCRLF(t *testing.T) {
	raw := "---\r\nkey: value\r\n---\r\nbody content"
	fm, body, ok := Split(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if fm != "key: value\r\n" {
		t.Errorf("fm = %q, want %q", fm, "key: value\r\n")
	}
	if body != "body content" {
		t.Errorf("body = %q, want %q", body, "body content")
	}
}

func TestSplitLeadingBOM(t *testing.T) {
	raw := "\ufeff---\nkey: value\n---\nbody"
	fm, body, ok := Split(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if fm != "key: value\n" {
		t.Errorf("fm = %q, want %q", fm, "key: value\n")
	}
	if body != "body" {
		t.Errorf("body = %q, want %q", body, "body")
	}
}

func TestSplitNoDelimiters(t *testing.T) {
	raw := "no frontmatter here at all"
	fm, body, ok := Split(raw)
	if ok {
		t.Fatalf("expected ok=false, got fm=%q body=%q", fm, body)
	}
	if fm != "" || body != "" {
		t.Errorf("expected empty fm/body on failure, got fm=%q body=%q", fm, body)
	}
}

func TestSplitNoClosingDelimiter(t *testing.T) {
	raw := "---\nkey: value\nno closing delimiter here"
	fm, body, ok := Split(raw)
	if ok {
		t.Fatalf("expected ok=false, got fm=%q body=%q", fm, body)
	}
	if fm != "" || body != "" {
		t.Errorf("expected empty fm/body on failure, got fm=%q body=%q", fm, body)
	}
}

func TestSplitEmptyFrontmatterBlock(t *testing.T) {
	raw := "---\n---\nbody"
	fm, body, ok := Split(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if fm != "" {
		t.Errorf("fm = %q, want empty", fm)
	}
	if body != "body" {
		t.Errorf("body = %q, want %q", body, "body")
	}
}

// --- DecodeStringList ------------------------------------------------------

// yamlNode parses a YAML document snippet and returns the top-level content
// node (i.e. what you'd get for a mapping value), mirroring how callers
// obtain a *yaml.Node from decoding a frontmatter field.
func yamlNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error: %v", src, err)
	}
	if len(doc.Content) != 1 {
		t.Fatalf("expected single document content node, got %d", len(doc.Content))
	}
	return doc.Content[0]
}

func TestDecodeStringListScalar(t *testing.T) {
	n := yamlNode(t, "login")
	var warned string
	got := DecodeStringList(n, func(msg string) { warned = msg })
	if len(got) != 1 || got[0] != "login" {
		t.Errorf("got %v, want [login]", got)
	}
	if warned != "was a scalar; wrapped into a list" {
		t.Errorf("warn message = %q", warned)
	}
}

func TestDecodeStringListSequence(t *testing.T) {
	n := yamlNode(t, "- a\n- b\n")
	warnCalled := false
	got := DecodeStringList(n, func(string) { warnCalled = true })
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want [a b]", got)
	}
	if warnCalled {
		t.Errorf("warn should not be called for a clean sequence")
	}
}

func TestDecodeStringListEmptyScalar(t *testing.T) {
	n := yamlNode(t, "\"\"\n")
	warnCalled := false
	got := DecodeStringList(n, func(string) { warnCalled = true })
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if warnCalled {
		t.Errorf("warn should not be called for an empty scalar")
	}
}

func TestDecodeStringListUnexpectedShape(t *testing.T) {
	n := yamlNode(t, "a: 1\nb: 2\n")
	var warned string
	got := DecodeStringList(n, func(msg string) { warned = msg })
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if warned != "has an unexpected YAML shape; ignored" {
		t.Errorf("warn message = %q", warned)
	}
}

func TestDecodeStringListSequenceWithNonScalarElement(t *testing.T) {
	n := yamlNode(t, "- a\n- b: 2\n")
	warnCalled := false
	got := DecodeStringList(n, func(string) { warnCalled = true })
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("got %v, want [a] (mapping element skipped)", got)
	}
	if warnCalled {
		t.Errorf("warn should not be called from within the sequence branch")
	}
}

// --- ParseTime / FormatTime -------------------------------------------------

func TestParseTimeLayouts(t *testing.T) {
	want := time.Date(2024, 3, 15, 9, 30, 45, 0, time.UTC)
	cases := []struct {
		name  string
		input string
	}{
		{"RFC3339", "2024-03-15T09:30:45Z"},
		{"DateTimeT", "2024-03-15T09:30:45"},
		{"DateTimeSpace", "2024-03-15 09:30:45"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseTime(c.input)
			if !got.Equal(want) {
				t.Errorf("ParseTime(%q) = %v, want %v", c.input, got, want)
			}
		})
	}
}

func TestParseTimeDateOnlyLayout(t *testing.T) {
	got := ParseTime("2024-03-15")
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseTime(date-only) = %v, want %v", got, want)
	}
}

func TestParseTimeRFC3339WithOffsetConvertsToUTC(t *testing.T) {
	got := ParseTime("2024-03-15T09:30:45+07:00")
	want := time.Date(2024, 3, 15, 2, 30, 45, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseTime(offset) = %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("expected UTC location, got %v", got.Location())
	}
}

func TestParseTimeEmpty(t *testing.T) {
	got := ParseTime("")
	if !got.IsZero() {
		t.Errorf("ParseTime(\"\") = %v, want zero time", got)
	}
	got = ParseTime("   ")
	if !got.IsZero() {
		t.Errorf("ParseTime(whitespace) = %v, want zero time", got)
	}
}

func TestParseTimeUnparseable(t *testing.T) {
	got := ParseTime("not-a-date-at-all")
	if !got.IsZero() {
		t.Errorf("ParseTime(garbage) = %v, want zero time", got)
	}
}

func TestFormatTimeZero(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "" {
		t.Errorf("FormatTime(zero) = %q, want empty", got)
	}
}

func TestFormatTimeNonZero(t *testing.T) {
	ts := time.Date(2024, 3, 15, 9, 30, 45, 0, time.UTC)
	got := FormatTime(ts)
	want := "2024-03-15T09:30:45Z"
	if got != want {
		t.Errorf("FormatTime = %q, want %q", got, want)
	}
}

func TestFormatTimeConvertsToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*60*60)
	ts := time.Date(2024, 3, 15, 16, 30, 45, 0, loc)
	got := FormatTime(ts)
	want := "2024-03-15T09:30:45Z"
	if got != want {
		t.Errorf("FormatTime(non-UTC) = %q, want %q", got, want)
	}
}

// --- Scalar / Seq ------------------------------------------------------------

func TestScalarPlainString(t *testing.T) {
	n := Scalar("plain")
	out, err := yaml.Marshal(n)
	if err != nil {
		t.Fatalf("yaml.Marshal error: %v", err)
	}
	if string(out) != "plain\n" {
		t.Errorf("marshaled = %q, want %q", string(out), "plain\n")
	}
}

func TestScalarQuotesSpecialString(t *testing.T) {
	// A value containing ": " looks like a mapping key/value pair, so YAML
	// must quote it to preserve it as a plain string scalar.
	n := Scalar("foo: bar")
	out, err := yaml.Marshal(n)
	if err != nil {
		t.Fatalf("yaml.Marshal error: %v", err)
	}
	if string(out) != "'foo: bar'\n" {
		t.Errorf("marshaled = %q, want %q", string(out), "'foo: bar'\n")
	}

	// Round-trip: decoding the quoted output must recover the original value.
	var got string
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}
	if got != "foo: bar" {
		t.Errorf("round-trip = %q, want %q", got, "foo: bar")
	}
}

func TestSeqBuildsBlockSequence(t *testing.T) {
	n := Seq([]string{"a", "b"})
	if n.Kind != yaml.SequenceNode {
		t.Fatalf("Kind = %v, want SequenceNode", n.Kind)
	}
	if len(n.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(n.Content))
	}
	out, err := yaml.Marshal(n)
	if err != nil {
		t.Fatalf("yaml.Marshal error: %v", err)
	}
	if string(out) != "- a\n- b\n" {
		t.Errorf("marshaled = %q, want %q", string(out), "- a\n- b\n")
	}
}

func TestSeqEmpty(t *testing.T) {
	n := Seq(nil)
	if n.Kind != yaml.SequenceNode {
		t.Fatalf("Kind = %v, want SequenceNode", n.Kind)
	}
	if len(n.Content) != 0 {
		t.Errorf("len(Content) = %d, want 0", len(n.Content))
	}
}
