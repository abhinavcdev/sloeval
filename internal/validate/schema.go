package validate

import (
	"bytes"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/spec"
	"github.com/abhinavcdev/sloeval/internal/suggest"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1/schema"
)

var (
	compileOnce sync.Once
	compiled    *jsonschema.Schema
	rawSchema   any
	compileErr  error
)

func loadSchema() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		rawSchema, compileErr = jsonschema.UnmarshalJSON(bytes.NewReader(schema.JSON))
		if compileErr != nil {
			return
		}
		c := jsonschema.NewCompiler()
		if compileErr = c.AddResource(schema.ID, rawSchema); compileErr != nil {
			return
		}
		compiled, compileErr = c.Compile(schema.ID)
	})
	return compiled, compileErr
}

// sliKinds lists the SLI kinds in schema order, for messages.
var sliKinds = []string{"latency", "errorRatio", "cost", "evalScore", "evalCoverage", "toolSuccess", "judgeAgreement"}

// requiredHints explains how to add a missing required field, by field name.
var requiredHints = map[string]string{
	"apiVersion":   "add apiVersion: " + v1alpha1.APIVersion,
	"kind":         "add kind: " + v1alpha1.Kind,
	"metadata":     "add metadata with a name, e.g. metadata: {name: support-agent}",
	"spec":         "add a spec with service and slos",
	"name":         "add a name using lowercase letters, digits and '-', e.g. name: ttft",
	"service":      "add service: <service name>, e.g. service: support-agent",
	"slos":         "add slos: a list with at least one SLO",
	"objective":    "add objective: the target percentage, e.g. objective: 99.5",
	"sli":          "add sli with exactly one of: " + strings.Join(sliKinds, ", "),
	"stage":        "add stage: ttft or end_to_end",
	"threshold":    "add threshold: a duration such as 2s or 500ms",
	"per":          "add per: request or task",
	"thresholdUSD": "add thresholdUSD: the maximum cost in US dollars, e.g. 0.05",
	"evaluator":    "add evaluator: the gen_ai.evaluation.name to use",
	"judge":        "add judge: the evaluator name of the LLM judge",
	"reference":    "add reference: the evaluator name of the reference labels, e.g. human_review",
	"long":         "add long: the long window, e.g. 1h",
	"short":        "add short: the short window, e.g. 5m",
	"burnRate":     "add burnRate: the burn-rate factor, e.g. 14.4",
}

// patternHints explains a pattern/length failure, by the schema location
// (fragment) of the failing subschema.
var patternHints = map[string]struct{ what, hint string }{
	"/$defs/duration":                 {"is not a valid duration", `use a duration such as "2s", "500ms", "2.56s", "1h30m" or "30d"`},
	"/$defs/dnsName":                  {"is not a valid name", "use 1-63 lowercase letters, digits and '-', starting and ending with a letter or digit, e.g. ttft or cost-per-task"},
	"/$defs/labelName":                {"is not a valid label name", "use letters, digits and '_', not starting with a digit, at most 64 characters"},
	"/$defs/labelValue":               {"is not allowed as a label value", "label values must be 1-128 characters of letters, digits and _ . : / - (no spaces); never put free text, prompts or IDs in labels"},
	"/$defs/alerting/properties/name": {"is not a valid alert name", "use CamelCase letters and digits, starting with a letter, e.g. SupportAgentTTFT"},
}

func fragment(schemaURL string) string {
	if i := strings.IndexByte(schemaURL, '#'); i >= 0 {
		return schemaURL[i+1:]
	}
	return ""
}

// schemaAt resolves a JSON pointer fragment in the raw schema.
func schemaAt(frag string) map[string]any {
	cur := rawSchema
	for _, tok := range strings.Split(strings.TrimPrefix(frag, "/"), "/") {
		if tok == "" {
			continue
		}
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[tok]
	}
	m, _ := cur.(map[string]any)
	return m
}

func allowedProperties(frag string) []string {
	props, _ := schemaAt(frag)["properties"].(map[string]any)
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// schemaChecker converts jsonschema errors into diagnostics.
type schemaChecker struct {
	doc   *spec.Document
	diags []diag.Diagnostic
}

func checkSchema(doc *spec.Document) ([]diag.Diagnostic, error) {
	sch, err := loadSchema()
	if err != nil {
		return nil, fmt.Errorf("internal error: compiling embedded schema: %w", err)
	}
	err = sch.Validate(doc.Value)
	if err == nil {
		return nil, nil
	}
	ve, ok := err.(*jsonschema.ValidationError) //nolint:errorlint // Validate returns this concrete type
	if !ok {
		return nil, err
	}
	c := &schemaChecker{doc: doc}
	c.walk(ve)
	return c.diags, nil
}

func (c *schemaChecker) add(p spec.Path, pos spec.Pos, msg, hint string) {
	c.diags = append(c.diags, diag.Diagnostic{
		Severity: diag.Error, File: c.doc.File, Document: c.doc.Index + 1,
		Path: p.String(), Line: pos.Line, Column: pos.Col, Message: msg, Hint: hint,
	})
}

// instancePath converts a jsonschema instance location to a spec.Path,
// using the value to tell sequence indexes from mapping keys.
func (c *schemaChecker) instancePath(loc []string) spec.Path {
	p := spec.Path{}
	cur := c.doc.Value
	for _, tok := range loc {
		switch v := cur.(type) {
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(v) {
				return p.Key(tok)
			}
			p = p.Index(i)
			cur = v[i]
		case map[string]any:
			p = p.Key(tok)
			cur = v[tok]
		default:
			return p.Key(tok)
		}
	}
	return p
}

func (c *schemaChecker) mapAt(p spec.Path) map[string]any {
	m, _ := c.valueAt(p).(map[string]any)
	return m
}

func (c *schemaChecker) requiredAt(frag string) []string {
	var out []string
	req, _ := schemaAt(frag)["required"].([]any)
	for _, r := range req {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// misspelled reports whether a missing required field of the object at p is
// probably present under a misspelled key.
func (c *schemaChecker) misspelled(p spec.Path, field string) bool {
	for k := range c.mapAt(p) {
		if k != field && suggest.Closest(k, []string{field}) == field {
			return true
		}
	}
	return false
}

func (c *schemaChecker) valueAt(p spec.Path) any {
	cur := c.doc.Value
	for _, s := range p {
		switch v := cur.(type) {
		case []any:
			if !s.IsIndex || s.Index >= len(v) {
				return nil
			}
			cur = v[s.Index]
		case map[string]any:
			cur = v[s.Key]
		default:
			return nil
		}
	}
	return cur
}

func (c *schemaChecker) walk(e *jsonschema.ValidationError) {
	switch k := e.ErrorKind.(type) {
	case *kind.OneOf:
		c.oneOf(e, k)
		return
	case *kind.Schema, *kind.Group, *kind.Reference, *kind.AllOf, *kind.AnyOf:
		for _, cause := range e.Causes {
			c.walk(cause)
		}
		return
	}
	if len(e.Causes) > 0 {
		for _, cause := range e.Causes {
			c.walk(cause)
		}
		return
	}
	c.leaf(e)
}

func (c *schemaChecker) oneOf(e *jsonschema.ValidationError, k *kind.OneOf) {
	p := c.instancePath(e.InstanceLocation)
	pos := c.doc.KeyPos(p)
	if strings.HasSuffix(fragment(e.SchemaURL), "/evalScore") {
		if len(k.Subschemas) > 1 {
			c.add(p, pos, "minScore and passLabels are both set", "set only one: minScore for numeric scores, or passLabels for label-based evaluators")
		} else {
			c.add(p, pos, "one of minScore or passLabels is required", "add minScore: 0.7 (score.value >= minScore passes) or passLabels: [pass]")
		}
		return
	}
	c.add(p, pos, "value must match exactly one of the allowed forms", "")
}

func describe(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case nil:
		return "null"
	case map[string]any:
		return "a mapping"
	case []any:
		return "a list"
	default:
		return fmt.Sprint(v)
	}
}

func ratString(r *big.Rat) string {
	f, _ := r.Float64()
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (c *schemaChecker) leaf(e *jsonschema.ValidationError) {
	p := c.instancePath(e.InstanceLocation)
	frag := fragment(e.SchemaURL)
	val := c.valueAt(p)
	pos := c.doc.Pos(p)

	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		for _, m := range k.Missing {
			if c.misspelled(p, m) {
				continue // reported as an unknown field with a "did you mean"
			}
			hint := requiredHints[m]
			if hint == "" {
				hint = fmt.Sprintf("add the %q field", m)
			}
			c.add(p.Key(m), c.doc.KeyPos(p), "required field is missing", hint)
		}

	case *kind.AdditionalProperties:
		allowed := allowedProperties(frag)
		for _, prop := range k.Properties {
			fp := p.Key(prop)
			msg := fmt.Sprintf("unknown field %q", prop)
			var hint string
			s := suggest.Closest(prop, allowed)
			switch {
			case s != "":
				hint = fmt.Sprintf("did you mean %q?", s)
				if _, present := c.mapAt(p)[s]; !present && slices.Contains(c.requiredAt(frag), s) {
					hint += fmt.Sprintf(" (%q is required)", s)
				}
			case strings.HasSuffix(frag, "/$defs/sli") && prop == "type":
				hint = "the SLI kind is a key, not a type field, e.g. sli: {latency: {stage: ttft, threshold: 2s}}"
			case len(allowed) > 0:
				hint = "allowed fields here: " + strings.Join(allowed, ", ")
			}
			c.add(fp, c.doc.KeyPos(fp), msg, hint)
		}

	case *kind.Type:
		got := k.Got
		if got == "integer" {
			got = "number"
		}
		msg := fmt.Sprintf("expected %s, got %s", strings.Join(k.Want, " or "), got)
		hint := ""
		switch {
		case slices.Contains(k.Want, "string") && (got == "number" || got == "boolean"):
			hint = fmt.Sprintf("quote the value to make it a string, e.g. %q", fmt.Sprint(val))
		case slices.Contains(k.Want, "number") && got == "string":
			hint = "remove the quotes and units; write a plain number"
		case slices.Contains(k.Want, "array"):
			hint = "write a list, e.g. [a, b] or one '- item' per line"
		case slices.Contains(k.Want, "object"):
			hint = "write a mapping of key: value pairs"
		}
		c.add(p, pos, msg, hint)

	case *kind.Const:
		want := describe(k.Want)
		hint := fmt.Sprintf("set it to %s", want)
		if p.String() == "apiVersion" {
			hint = fmt.Sprintf("this version of sloe supports apiVersion %s", want)
		} else if p.String() == "kind" {
			hint = fmt.Sprintf("set kind: %s; sloe validate only accepts AISLO documents, so keep other resources in separate files", v1alpha1.Kind)
		}
		c.add(p, pos, fmt.Sprintf("must be %s, got %s", want, describe(k.Got)), hint)

	case *kind.Enum:
		opts := make([]string, 0, len(k.Want))
		for _, w := range k.Want {
			opts = append(opts, fmt.Sprint(w))
		}
		hint := "use one of: " + strings.Join(opts, ", ")
		if s, ok := k.Got.(string); ok {
			if sug := suggest.Closest(s, opts); sug != "" {
				hint = fmt.Sprintf("did you mean %q? %s", sug, hint)
			}
		}
		c.add(p, pos, fmt.Sprintf("%s is not a valid value", describe(k.Got)), hint)

	case *kind.Pattern:
		c.patternLike(p, pos, frag, val, "")

	case *kind.MinLength:
		c.patternLike(p, pos, frag, val, "is empty")

	case *kind.MaxLength:
		c.patternLike(p, pos, frag, val, "is too long")

	case *kind.ExclusiveMinimum, *kind.ExclusiveMaximum, *kind.Minimum, *kind.Maximum:
		c.numberRange(p, pos, e.ErrorKind, val)

	case *kind.MinItems:
		if p.String() == "spec.slos" {
			c.add(p, pos, "at least one SLO is required", requiredHints["slos"])
		} else {
			c.add(p, pos, "list must not be empty", "add at least one item, or remove the field")
		}

	case *kind.MinProperties:
		if strings.HasSuffix(frag, "/$defs/sli") {
			c.add(p, pos, "no SLI kind is set", "set exactly one of: "+strings.Join(sliKinds, ", "))
		} else {
			c.add(p, pos, "mapping must not be empty", "")
		}

	case *kind.MaxProperties:
		if strings.HasSuffix(frag, "/$defs/sli") {
			var set []string
			if m, ok := val.(map[string]any); ok {
				for _, k := range sliKinds {
					if _, ok := m[k]; ok {
						set = append(set, k)
					}
				}
			}
			c.add(p, pos, "more than one SLI kind is set ("+strings.Join(set, ", ")+")",
				"an SLO measures one SLI; split it into separate SLOs, one per kind")
		} else {
			c.add(p, pos, "mapping has too many fields", "")
		}

	default:
		c.add(p, pos, fmt.Sprintf("schema check %q failed", strings.Join(e.ErrorKind.KeywordPath(), "/")), "")
	}
}

func (c *schemaChecker) patternLike(p spec.Path, pos spec.Pos, frag string, val any, fallback string) {
	h, ok := patternHints[frag]
	if !ok {
		what := fallback
		if what == "" {
			what = "has an invalid format"
		}
		c.add(p, pos, fmt.Sprintf("%s %s", describe(val), what), "")
		return
	}
	c.add(p, pos, fmt.Sprintf("%s %s", describe(val), h.what), h.hint)
}

func (c *schemaChecker) numberRange(p spec.Path, pos spec.Pos, k jsonschema.ErrorKind, val any) {
	last := ""
	if len(p) > 0 {
		last = p[len(p)-1].Key
	}
	switch last {
	case "objective":
		c.add(p, pos, fmt.Sprintf("objective must be greater than 0 and less than 100, got %v", val),
			"objective is a percentage, e.g. 99.5 for 99.5%")
		return
	case "thresholdUSD":
		c.add(p, pos, fmt.Sprintf("thresholdUSD must be greater than 0, got %v", val), "use the maximum acceptable cost in US dollars, e.g. 0.05")
		return
	case "burnRate":
		c.add(p, pos, fmt.Sprintf("burnRate must be greater than 0, got %v", val), "use a burn-rate factor such as 14.4 or 6")
		return
	}
	var msg string
	switch k := k.(type) {
	case *kind.ExclusiveMinimum:
		msg = "must be greater than " + ratString(k.Want)
	case *kind.ExclusiveMaximum:
		msg = "must be less than " + ratString(k.Want)
	case *kind.Minimum:
		msg = "must be at least " + ratString(k.Want)
	case *kind.Maximum:
		msg = "must be at most " + ratString(k.Want)
	}
	c.add(p, pos, fmt.Sprintf("%s, got %v", msg, val), "")
}
