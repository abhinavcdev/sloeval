// Package validate checks AISLO documents: first against the JSON Schema,
// then with semantic checks the schema cannot express. Every problem is a
// diagnostic that names the field and, where possible, suggests a fix.
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/spec"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

// Input is one file to validate.
type Input struct {
	Name string
	Data []byte
}

// Doc is a loaded document. AISLO is nil if the document could not be
// decoded into the typed API (because of schema errors).
type Doc struct {
	Source *spec.Document
	AISLO  *v1alpha1.AISLO
}

// Result of validating a set of files.
type Result struct {
	Docs        []Doc
	Diagnostics []diag.Diagnostic
}

// OK reports whether there are no errors (warnings are allowed).
func (r Result) OK() bool {
	errs, _ := diag.Count(r.Diagnostics)
	return errs == 0
}

// Run validates all inputs. Diagnostics are sorted.
func Run(inputs []Input) (Result, error) {
	var res Result
	type id struct{ ns, name string }
	owners := map[id]string{}
	for _, in := range inputs {
		docs, diags := spec.Load(in.Name, in.Data)
		res.Diagnostics = append(res.Diagnostics, diags...)
		for _, d := range docs {
			doc, ds, err := one(d)
			if err != nil {
				return Result{}, err
			}
			res.Docs = append(res.Docs, doc)
			res.Diagnostics = append(res.Diagnostics, ds...)
			if doc.AISLO == nil || doc.AISLO.Metadata.Name == "" {
				continue
			}
			k := id{doc.AISLO.Metadata.Namespace, doc.AISLO.Metadata.Name}
			p := spec.Path{}.Key("metadata").Key("name")
			pos := d.Pos(p)
			here := fmt.Sprintf("%s:%d", d.File, pos.Line)
			if first, dup := owners[k]; dup {
				res.Diagnostics = append(res.Diagnostics, diag.Diagnostic{
					Severity: diag.Error, File: d.File, Document: d.Index + 1, Path: p.String(),
					Line: pos.Line, Column: pos.Col,
					Message: fmt.Sprintf("duplicate AISLO %q (first defined at %s)", k.name, first),
					Hint:    "metadata.name must be unique per namespace; rename one of them",
				})
			} else {
				owners[k] = here
			}
		}
	}
	diag.Sort(res.Diagnostics)
	return res, nil
}

func one(d *spec.Document) (Doc, []diag.Diagnostic, error) {
	schemaDiags, err := checkSchema(d)
	if err != nil {
		return Doc{}, nil, err
	}
	doc := Doc{Source: d}
	obj, ok := decode(d.Value)
	if !ok {
		return doc, schemaDiags, nil
	}
	doc.AISLO = obj
	// A field with a schema error is not checked again semantically.
	bad := map[string]bool{}
	for _, sd := range schemaDiags {
		bad[sd.Path] = true
	}
	out := schemaDiags
	for _, sd := range checkSemantics(d, obj) {
		if !bad[sd.Path] {
			out = append(out, sd)
		}
	}
	return doc, out, nil
}

// decode converts a generic value into the typed API. It fails if a value
// has the wrong type, which the schema pass has already reported.
func decode(v any) (*v1alpha1.AISLO, bool) {
	if _, ok := v.(map[string]any); !ok {
		return nil, false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var obj v1alpha1.AISLO
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&obj); err != nil {
		return nil, false
	}
	return &obj, true
}
