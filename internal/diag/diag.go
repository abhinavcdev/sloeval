// Package diag defines diagnostics reported by sloe and how they are printed.
package diag

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Severity of a diagnostic.
type Severity string

// Severities.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Diagnostic is one problem found in a spec file.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	// Document is the 1-based index of the YAML document in the file (0 if unknown).
	Document int `json:"document,omitempty"`
	// Path is the field path, e.g. spec.slos[0].sli.latency.threshold.
	Path string `json:"path,omitempty"`
	// Line and Column are 1-based; 0 means unknown.
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Sort orders diagnostics by file, position, path and message, so output is deterministic.
func Sort(ds []Diagnostic) {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Message, b.Message),
		)
	})
}

// Count returns the number of errors and warnings.
func Count(ds []Diagnostic) (errs, warns int) {
	for _, d := range ds {
		switch d.Severity {
		case Error:
			errs++
		case Warning:
			warns++
		}
	}
	return errs, warns
}

// String renders d on one line (plus an indented hint line), compiler style:
//
//	file:line:col: error: path: message
//	    hint: ...
func (d Diagnostic) String() string {
	var sb strings.Builder
	sb.WriteString(d.File)
	if d.Line > 0 {
		fmt.Fprintf(&sb, ":%d", d.Line)
		if d.Column > 0 {
			fmt.Fprintf(&sb, ":%d", d.Column)
		}
	}
	fmt.Fprintf(&sb, ": %s: ", d.Severity)
	if d.Path != "" {
		sb.WriteString(d.Path)
		sb.WriteString(": ")
	}
	sb.WriteString(d.Message)
	if d.Hint != "" {
		sb.WriteString("\n    hint: ")
		sb.WriteString(d.Hint)
	}
	return sb.String()
}

// WriteText writes diagnostics one per entry.
func WriteText(w io.Writer, ds []Diagnostic) error {
	for _, d := range ds {
		if _, err := fmt.Fprintln(w, d.String()); err != nil {
			return err
		}
	}
	return nil
}

// Report is the JSON output of a validate run.
type Report struct {
	Files       []FileReport `json:"files"`
	Errors      int          `json:"errors"`
	Warnings    int          `json:"warnings"`
	Valid       bool         `json:"valid"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// FileReport summarizes one file.
type FileReport struct {
	File      string `json:"file"`
	Documents int    `json:"documents"`
	SLOs      int    `json:"slos"`
	Errors    int    `json:"errors"`
	Warnings  int    `json:"warnings"`
}

// WriteJSON writes r as indented JSON.
func WriteJSON(w io.Writer, r Report) error {
	if r.Diagnostics == nil {
		r.Diagnostics = []Diagnostic{}
	}
	if r.Files == nil {
		r.Files = []FileReport{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
