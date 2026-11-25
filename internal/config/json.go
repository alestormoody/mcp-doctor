package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// JSONDoc is a parsed JSON or JSONC document. Positions refer to the BOM-stripped buffer.
type JSONDoc struct {
	BOM  bool
	body []byte       // prepared buffer (BOM stripped, maybe "\n" appended)
	std  []byte       // standardized copy of body: same length, comments and trailing commas blanked
	root hujson.Value // original AST (never mutated)
}

// Member is one JSON value; for object members Name is the key.
type Member struct {
	Name       string
	Line, Col  int // key position for object members; value position for Root()
	Start, End int // value byte span in the prepared buffer
	doc        *JSONDoc
	v          *hujson.Value
}

// ParseJSON parses JSON or JSONC. It never returns an error: problems are data, and doc is nil iff a
// KindSyntax problem is returned. strict reports comments and trailing commas as KindNonStandard.
func ParseJSON(src []byte, strict bool) (*JSONDoc, []Problem) {
	body, hasBOM, _ := prepare(src)
	var ps []Problem
	if hasBOM {
		ps = append(ps, Problem{Kind: KindBOM, Line: 1, Col: 1, Message: MsgBOM})
	}
	if len(bytes.Trim(body, " \t\r\n")) == 0 {
		return nil, append(ps, Problem{Kind: KindSyntax, Line: 1, Col: 1, Message: MsgEmpty})
	}
	root, err := hujson.Parse(body)
	if err != nil {
		p := Problem{Kind: KindSyntax, Message: syntaxMessage(err.Error())}
		if _, serr := fmt.Sscanf(err.Error(), "hujson: line %d, column %d:", &p.Line, &p.Col); serr != nil {
			p.Line, p.Col = 0, 0
		}
		return nil, append(ps, p)
	}
	// Standardize a fresh parse of a copy: Clone drops trailing commas (shifting offsets) and
	// Standardize blanks comments in place in the parsed buffer.
	work, err := hujson.Parse(bytes.Clone(body))
	if err != nil {
		return nil, append(ps, Problem{Kind: KindSyntax, Message: MsgSyntax}) // unreachable: same bytes parsed above
	}
	work.Standardize()
	d := &JSONDoc{BOM: hasBOM, body: body, std: work.Pack(), root: root}
	if err := json.Unmarshal(d.std, new(json.RawMessage)); err != nil { // encoding/json caps depth at 10 000
		return nil, append(ps, d.syntaxProblem(0, err))
	}
	if strict && !root.IsStandard() {
		// The first byte Standardize changed is the first comment or trailing comma.
		i := 0
		for i < len(body) && body[i] == d.std[i] {
			i++
		}
		line, col := LineCol(body, i)
		ps = append(ps, Problem{Kind: KindNonStandard, Line: line, Col: col, Message: MsgNonStandard})
	}
	return d, ps
}

// syntaxProblem positions an encoding/json error found while reading bytes that start at base.
func (d *JSONDoc) syntaxProblem(base int, err error) Problem {
	p := Problem{Kind: KindSyntax, Message: syntaxMessage(err.Error())}
	off := base
	if se := (*json.SyntaxError)(nil); errors.As(err, &se) {
		off += max(0, int(se.Offset)-1) // Offset counts bytes read: the offending byte is the last one
	}
	p.Line, p.Col = LineCol(d.body, off)
	return p
}

// Root returns the document's top-level value.
func (d *JSONDoc) Root() Member {
	return d.member("", &d.root, d.root.StartOffset)
}

func (d *JSONDoc) member(name string, v *hujson.Value, pos int) Member {
	line, col := LineCol(d.body, pos)
	return Member{Name: name, Line: line, Col: col, Start: v.StartOffset, End: v.EndOffset, doc: d, v: v}
}

// IsObject reports whether the value is a JSON object.
func (m Member) IsObject() bool {
	_, ok := m.v.Value.(*hujson.Object)
	return ok
}

// Members lists object members in document order; isObject is false for non-objects. A repeated key
// yields one KindDuplicate problem per repeat, and only its last occurrence is returned, at that
// position (JavaScript JSON.parse semantics).
func (m Member) Members() (ms []Member, isObject bool, problems []Problem) {
	obj, ok := m.v.Value.(*hujson.Object)
	if !ok {
		return nil, false, nil
	}
	all := make([]Member, len(obj.Members))
	last := map[string]int{}
	for i := range obj.Members {
		om := &obj.Members[i]
		name := om.Name.Value.(hujson.Literal).String() // object names are always string literals
		all[i] = m.doc.member(name, &om.Value, om.Name.StartOffset)
		if j, dup := last[name]; dup {
			problems = append(problems, Problem{
				Kind: KindDuplicate, Line: all[i].Line, Col: all[i].Col, Message: MsgDuplicate, Key: name,
				PrevLine: all[j].Line, PrevCol: all[j].Col,
			})
		}
		last[name] = i
	}
	ms = make([]Member, 0, len(last))
	for i, x := range all {
		if last[x.Name] == i {
			ms = append(ms, x)
		}
	}
	return ms, true, problems
}

// Decode decodes the value's standardized bytes with encoding/json (UseNumber) into v. A problem is
// positioned in the file; its message is fixed and never carries error text.
func (m Member) Decode(v any) *Problem {
	dec := json.NewDecoder(bytes.NewReader(m.doc.std[m.Start:m.End]))
	dec.UseNumber()
	err := dec.Decode(v)
	if err == nil {
		return nil
	}
	if te := (*json.UnmarshalTypeError)(nil); errors.As(err, &te) {
		off := m.Start // the fragment itself has the wrong type
		if te.Field != "" {
			off += int(te.Offset) // shortcut: Offset is the end of the bad field value; callers decode one field per Member
		}
		line, col := LineCol(m.doc.body, off)
		return &Problem{Kind: KindType, Line: line, Col: col, Message: MsgType}
	}
	p := m.doc.syntaxProblem(m.Start, err)
	return &p
}

// Bytes returns the original bytes of the value, comments included. Callers must not mutate it.
func (m Member) Bytes() []byte { return m.doc.body[m.Start:m.End] }
