// Package spec loads AISLO YAML files into generic values, keeping the line
// and column of every field so diagnostics can point at the source.
package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/abhinavcdev/sloeval/internal/diag"
)

// Pos is a 1-based source position.
type Pos struct{ Line, Col int }

// Document is one YAML document converted to JSON-compatible values
// (map[string]any, []any, string, int64, float64, bool, nil).
type Document struct {
	File string
	// Index is the 0-based position of the document in the file.
	Index int
	Start Pos
	Value any

	values map[string]Pos
	keys   map[string]Pos
}

// Pos returns the position of the value at path, or of its nearest ancestor
// that exists.
func (d *Document) Pos(p Path) Pos {
	for {
		if pos, ok := d.values[p.String()]; ok {
			return pos
		}
		if len(p) == 0 {
			return d.Start
		}
		p = p.Parent()
	}
}

// KeyPos returns the position of the mapping key that introduces path (for
// "unknown field" errors), falling back to Pos.
func (d *Document) KeyPos(p Path) Pos {
	if pos, ok := d.keys[p.String()]; ok {
		return pos
	}
	return d.Pos(p)
}

// maxNodes bounds alias expansion ("billion laughs").
const maxNodes = 100_000

// Load parses every YAML document in data. Empty documents are skipped.
// Problems that prevent loading a document are returned as diagnostics.
func Load(file string, data []byte) ([]*Document, []diag.Diagnostic) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*Document
	var diags []diag.Diagnostic
	for i := 0; ; i++ {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			diags = append(diags, syntaxError(file, i+1, err))
			break
		}
		if len(n.Content) == 0 {
			continue
		}
		root := n.Content[0]
		if root.Kind == yaml.ScalarNode && root.ShortTag() == "!!null" && root.Value == "" {
			continue // empty document, e.g. between two '---'
		}
		d := &Document{
			File:   file,
			Index:  i,
			Start:  Pos{root.Line, root.Column},
			values: map[string]Pos{},
			keys:   map[string]Pos{},
		}
		c := converter{doc: d}
		d.Value = c.convert(root, nil)
		docs = append(docs, d)
		diags = append(diags, c.diags...)
	}
	if len(docs) == 0 && len(diags) == 0 {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error, File: file,
			Message: "file contains no YAML documents",
			Hint:    "add an AISLO document starting with apiVersion: sloeval.dev/v1alpha1 and kind: AISLO",
		})
	}
	return docs, diags
}

var lineRe = regexp.MustCompile(`line (\d+)`)

func syntaxError(file string, doc int, err error) diag.Diagnostic {
	d := diag.Diagnostic{
		Severity: diag.Error, File: file, Document: doc,
		Message: "invalid YAML: " + err.Error(),
		Hint:    "fix the YAML syntax; check indentation and that every key is followed by ': '",
	}
	if m := lineRe.FindStringSubmatch(err.Error()); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
	}
	return d
}

type converter struct {
	doc   *Document
	diags []diag.Diagnostic
	nodes int
}

func (c *converter) errorf(n *yaml.Node, p Path, hint, format string, args ...any) {
	c.diags = append(c.diags, diag.Diagnostic{
		Severity: diag.Error, File: c.doc.File, Document: c.doc.Index + 1,
		Path: p.String(), Line: n.Line, Column: n.Column,
		Message: fmt.Sprintf(format, args...), Hint: hint,
	})
}

func (c *converter) convert(n *yaml.Node, p Path) any {
	c.nodes++
	if c.nodes > maxNodes {
		if c.nodes == maxNodes+1 {
			c.errorf(n, p, "simplify the document; deep alias expansion is not allowed",
				"document is too large (more than %d nodes after alias expansion)", maxNodes)
		}
		return nil
	}
	c.doc.values[p.String()] = Pos{n.Line, n.Column}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return c.convert(n.Content[0], p)
	case yaml.AliasNode:
		v := c.convert(n.Alias, p)
		c.doc.values[p.String()] = Pos{n.Line, n.Column}
		return v
	case yaml.MappingNode:
		return c.mapping(n, p)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for i, item := range n.Content {
			out = append(out, c.convert(item, p.Index(i)))
		}
		return out
	case yaml.ScalarNode:
		return c.scalar(n, p)
	}
	c.errorf(n, p, "", "unsupported YAML node")
	return nil
}

func (c *converter) mapping(n *yaml.Node, p Path) any {
	out := make(map[string]any, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind == yaml.AliasNode {
			k = k.Alias
		}
		if k.Tag == "!!merge" {
			c.errorf(k, p, "write the fields out in full instead of using '<<'",
				"YAML merge keys are not supported")
			continue
		}
		if k.Kind != yaml.ScalarNode || k.Tag == "!!null" {
			c.errorf(k, p, "use a plain string as the key", "mapping keys must be strings")
			continue
		}
		kp := p.Key(k.Value)
		if _, dup := out[k.Value]; dup {
			first := c.doc.keys[kp.String()]
			c.errorf(k, kp, fmt.Sprintf("remove one of them (first defined at line %d)", first.Line),
				"duplicate key %q", k.Value)
			continue
		}
		c.doc.keys[kp.String()] = Pos{k.Line, k.Column}
		out[k.Value] = c.convert(v, kp)
	}
	return out
}

func (c *converter) scalar(n *yaml.Node, p Path) any {
	switch n.ShortTag() {
	case "!!str", "!!timestamp", "!!binary":
		return n.Value
	case "!!null":
		return nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			c.errorf(n, p, "use true or false", "invalid boolean %q", n.Value)
			return nil
		}
		return b
	case "!!int":
		var i int64
		if err := n.Decode(&i); err != nil {
			c.errorf(n, p, "use a smaller number, or quote it to make it a string", "invalid integer %q", n.Value)
			return nil
		}
		return i
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil {
			c.errorf(n, p, "use a decimal number such as 99.5", "invalid number %q", n.Value)
			return nil
		}
		if math.IsInf(f, 0) || math.IsNaN(f) {
			c.errorf(n, p, "use a finite decimal number", "%q is not a finite number", n.Value)
			return nil
		}
		return f
	}
	c.errorf(n, p, "remove the tag; custom YAML tags are not supported", "unsupported YAML tag %q", n.Tag)
	return nil
}
