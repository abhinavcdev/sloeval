package spec

import (
	"reflect"
	"strings"
	"testing"
)

func TestPathString(t *testing.T) {
	p := Path{}.Key("spec").Key("slos").Index(2).Key("sli")
	if got, want := p.String(), "spec.slos[2].sli"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	q := Path{}.Key("metadata").Key("annotations").Key("example.com/x")
	if got, want := q.String(), `metadata.annotations["example.com/x"]`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := (Path{}).String(); got != "" {
		t.Errorf("root path = %q", got)
	}
}

func TestLoadValuesAndPositions(t *testing.T) {
	src := `a: 1
b:
  - x
  - y: 2.5
c: true
d: ~
e: "1"
f: yes
`
	docs, diags := Load("t.yaml", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs", len(docs))
	}
	want := map[string]any{
		"a": int64(1),
		"b": []any{"x", map[string]any{"y": 2.5}},
		"c": true,
		"d": nil,
		"e": "1",
		"f": "yes", // YAML 1.2: yes is a string
	}
	if !reflect.DeepEqual(docs[0].Value, want) {
		t.Errorf("value = %#v\nwant   %#v", docs[0].Value, want)
	}
	tests := []struct {
		path       Path
		value, key Pos
	}{
		{Path{}.Key("a"), Pos{1, 4}, Pos{1, 1}},
		{Path{}.Key("b").Index(1).Key("y"), Pos{4, 8}, Pos{4, 5}},
		{Path{}.Key("b").Index(1).Key("missing"), Pos{4, 5}, Pos{4, 5}},
	}
	for _, tt := range tests {
		if got := docs[0].Pos(tt.path); got != tt.value {
			t.Errorf("Pos(%s) = %v, want %v", tt.path, got, tt.value)
		}
		if got := docs[0].KeyPos(tt.path); got != tt.key {
			t.Errorf("KeyPos(%s) = %v, want %v", tt.path, got, tt.key)
		}
	}
}

func TestLoadMultiDoc(t *testing.T) {
	docs, diags := Load("t.yaml", []byte("a: 1\n---\n---\nb: 2\n"))
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	if len(docs) != 2 || docs[0].Index != 0 || docs[1].Index != 2 || docs[1].Start.Line != 4 {
		t.Fatalf("docs = %+v", docs)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, src, msg string
		line           int
	}{
		{"syntax", "a: 1\n b: 2\n", "invalid YAML", 2},
		{"duplicate", "a: 1\na: 2\n", `duplicate key "a"`, 2},
		{"merge", "x: &x {a: 1}\ny:\n  <<: *x\n", "merge keys", 3},
		{"tag", "a: !custom 1\n", "unsupported YAML tag", 1},
		{"inf", "a: .inf\n", "not a finite number", 1},
		{"empty", "", "no YAML documents", 0},
		{"nullkey", "~: 1\n", "keys must be strings", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := Load("t.yaml", []byte(tt.src))
			if len(diags) == 0 {
				t.Fatal("expected a diagnostic")
			}
			if !strings.Contains(diags[0].Message, tt.msg) {
				t.Errorf("message %q does not contain %q", diags[0].Message, tt.msg)
			}
			if diags[0].Line != tt.line {
				t.Errorf("line = %d, want %d", diags[0].Line, tt.line)
			}
		})
	}
}

func TestLoadAliasBomb(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 8; i++ {
		sb.WriteString("a" + string(rune('0'+i)) + ": &a" + string(rune('0'+i)) + " [")
		for j := 0; j < 10; j++ {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("*a" + string(rune('0'+i-1)))
		}
		sb.WriteString("]\n")
	}
	_, diags := Load("t.yaml", []byte(sb.String()))
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "too large") {
		t.Fatalf("diags = %v", diags)
	}
}
