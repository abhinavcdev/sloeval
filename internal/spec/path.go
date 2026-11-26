package spec

import (
	"regexp"
	"strconv"
	"strings"
)

// Segment is one step in a Path: a mapping key or a sequence index.
type Segment struct {
	Key     string
	Index   int
	IsIndex bool
}

// Path locates a value in a document, e.g. spec.slos[0].sli.
type Path []Segment

// Key returns a new path with a mapping key appended.
func (p Path) Key(k string) Path {
	return append(p[:len(p):len(p)], Segment{Key: k})
}

// Index returns a new path with a sequence index appended.
func (p Path) Index(i int) Path {
	return append(p[:len(p):len(p)], Segment{Index: i, IsIndex: true})
}

// Parent returns the path without its last segment.
func (p Path) Parent() Path {
	if len(p) == 0 {
		return p
	}
	return p[: len(p)-1 : len(p)-1]
}

var plainKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// String renders the path as spec.slos[0].sli.latency; keys that are not
// plain identifiers are quoted: metadata.annotations["example.com/x"].
func (p Path) String() string {
	var sb strings.Builder
	for i, s := range p {
		switch {
		case s.IsIndex:
			sb.WriteString("[" + strconv.Itoa(s.Index) + "]")
		case plainKey.MatchString(s.Key):
			if i > 0 {
				sb.WriteByte('.')
			}
			sb.WriteString(s.Key)
		default:
			sb.WriteString("[" + strconv.Quote(s.Key) + "]")
		}
	}
	return sb.String()
}
