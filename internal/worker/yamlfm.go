package worker

import (
	"errors"
	"fmt"
	"strings"
)

// yamlfm.go parses the YAML frontmatter of Sinet worker template and skill
// files. The template file format is "markdown + YAML frontmatter" in a
// schema Sinet OWNS (Spec S08.1), so what is accepted here is a strict,
// closed YAML subset — exactly the shapes the schema needs — parsed by
// Sinet's own code (stdlib-first, CONVENTIONS §2: a YAML library would be
// an adoption). Anything outside the subset fails loudly; a definition
// file is untrusted content (Spec S08.10, P-T14-2) and fail-closed parsing
// is part of its screening.
//
// The subset:
//
//   - document:   "---\n" <yaml lines> "---\n" <markdown body>
//   - top level:  "key: value" | "key: [a, b]" | "key:" + indented block
//   - block:      2-space-indented "key: value" / "key: [a, b]" lines
//     (a nested map, one level deep) or "- item" lines (a list)
//   - scalars:    plain or single-/double-quoted strings; "#" starts a
//     comment (full-line, or trailing after an unquoted scalar)
//   - keys:       [A-Za-z0-9_-]+, unique per map
//
// Tabs, deeper nesting, flow maps, anchors, multi-line scalars, and
// duplicate keys are rejected.

// fmVal is one parsed frontmatter value: exactly one of Scalar (with
// IsScalar), List, or Map is meaningful.
type fmVal struct {
	IsScalar bool
	Scalar   string
	IsList   bool
	List     []string
	Map      *fmMap
}

// fmMap is an order-preserving parsed map.
type fmMap struct {
	keys []string
	vals map[string]fmVal
}

func newFmMap() *fmMap { return &fmMap{vals: map[string]fmVal{}} }

func (m *fmMap) set(key string, v fmVal) error {
	if _, dup := m.vals[key]; dup {
		return fmt.Errorf("duplicate key %q", key)
	}
	m.keys = append(m.keys, key)
	m.vals[key] = v
	return nil
}

func (m *fmMap) get(key string) (fmVal, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// keyPaths returns every key path in the map ("profile.duty" style), the
// walk surface of the guardrail-field lint (Spec S08.2: a guardrail-class
// field appearing ANYWHERE in a definition file is a structural reject).
func (m *fmMap) keyPaths(prefix string) []string {
	var out []string
	for _, k := range m.keys {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		out = append(out, p)
		if v := m.vals[k]; v.Map != nil {
			out = append(out, v.Map.keyPaths(p)...)
		}
	}
	return out
}

var errNoFrontmatter = errors.New("missing frontmatter delimiter")

// splitFrontmatter splits a template/skill document into raw frontmatter
// lines and the markdown body.
func splitFrontmatter(src string) (fm []string, body string, err error) {
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " ") != "---" {
		return nil, "", errNoFrontmatter
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " ") == "---" {
			return lines[1:i], strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return nil, "", errors.New("unterminated frontmatter (no closing ---)")
}

func validFmKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// parseScalar interprets one scalar token: quoted (verbatim inside quotes)
// or plain (trailing comment stripped, whitespace trimmed).
func parseScalar(raw string, line int) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if s[0] == '"' || s[0] == '\'' {
		q := s[0]
		end := strings.IndexByte(s[1:], q)
		if end < 0 {
			return "", fmt.Errorf("line %d: unterminated quoted scalar", line)
		}
		rest := strings.TrimSpace(s[end+2:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("line %d: trailing content after quoted scalar", line)
		}
		return s[1 : end+1], nil
	}
	if i := strings.Index(s, " #"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if strings.HasPrefix(s, "#") {
		return "", nil
	}
	return s, nil
}

// parseFlowList parses "[a, b, c]".
func parseFlowList(raw string, line int) ([]string, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, " #"); i >= 0 && !strings.Contains(s[:i], "]") {
		return nil, fmt.Errorf("line %d: comment inside flow list", line)
	}
	if !strings.HasSuffix(s, "]") {
		// Allow a trailing comment after the closing bracket.
		if j := strings.LastIndexByte(s, ']'); j >= 0 && strings.HasPrefix(strings.TrimSpace(s[j+1:]), "#") {
			s = s[:j+1]
		} else {
			return nil, fmt.Errorf("line %d: unterminated flow list", line)
		}
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return []string{}, nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v, err := parseScalar(p, line)
		if err != nil {
			return nil, err
		}
		if v == "" {
			return nil, fmt.Errorf("line %d: empty element in flow list", line)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseFrontmatter parses the raw frontmatter lines into the ordered map.
func parseFrontmatter(lines []string) (*fmMap, error) {
	top := newFmMap()
	i := 0
	for i < len(lines) {
		line := lines[i]
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		if strings.ContainsRune(line, '\t') {
			return nil, fmt.Errorf("line %d: tabs are not accepted in frontmatter", lineNo)
		}
		if line[0] == ' ' {
			return nil, fmt.Errorf("line %d: unexpected indentation", lineNo)
		}
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"key: value\"", lineNo)
		}
		key = strings.TrimSpace(key)
		if !validFmKey(key) {
			return nil, fmt.Errorf("line %d: invalid key %q", lineNo, key)
		}
		val, consumed, err := parseValue(rest, lines, i, lineNo, false)
		if err != nil {
			return nil, err
		}
		if err := top.set(key, val); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		i += consumed
	}
	return top, nil
}

// parseValue parses the value after "key:" at lines[start]; rest is the
// remainder of the key line. It returns how many lines were consumed
// (≥ 1). nested marks parsing inside an indented block, where further
// nesting is rejected (the subset is two levels deep).
func parseValue(rest string, lines []string, start, lineNo int, nested bool) (fmVal, int, error) {
	trimmedRest := strings.TrimSpace(rest)
	if trimmedRest != "" && !strings.HasPrefix(trimmedRest, "#") {
		if strings.HasPrefix(trimmedRest, "[") {
			list, err := parseFlowList(trimmedRest, lineNo)
			if err != nil {
				return fmVal{}, 0, err
			}
			return fmVal{IsList: true, List: list}, 1, nil
		}
		if strings.HasPrefix(trimmedRest, "{") {
			return fmVal{}, 0, fmt.Errorf("line %d: flow maps are not accepted", lineNo)
		}
		s, err := parseScalar(trimmedRest, lineNo)
		if err != nil {
			return fmVal{}, 0, err
		}
		if s == "" {
			// The scalar was only a comment: treat as an empty value.
			return fmVal{IsScalar: true}, 1, nil
		}
		return fmVal{IsScalar: true, Scalar: s}, 1, nil
	}

	// Block form: collect the indented lines that follow.
	if nested {
		return fmVal{}, 0, fmt.Errorf("line %d: nesting deeper than one block is not accepted", lineNo)
	}
	var block []string
	consumed := 1
	for j := start + 1; j < len(lines); j++ {
		l := lines[j]
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			// Blank/comment lines inside a block are consumed only when
			// more block lines follow; decided below by lookahead.
			if k := nextContent(lines, j); k >= 0 && strings.HasPrefix(lines[k], "  ") {
				consumed++
				continue
			}
			break
		}
		if strings.ContainsRune(l, '\t') {
			return fmVal{}, 0, fmt.Errorf("line %d: tabs are not accepted in frontmatter", j+1)
		}
		if !strings.HasPrefix(l, "  ") {
			break
		}
		if strings.HasPrefix(l, "   ") && !strings.HasPrefix(strings.TrimPrefix(l, "  "), "- ") {
			return fmVal{}, 0, fmt.Errorf("line %d: indentation must be exactly two spaces", j+1)
		}
		block = append(block, strings.TrimPrefix(l, "  "))
		consumed++
	}
	if len(block) == 0 {
		return fmVal{IsScalar: true}, 1, nil
	}

	if strings.HasPrefix(strings.TrimSpace(block[0]), "- ") || strings.TrimSpace(block[0]) == "-" {
		// Block list of scalars.
		var list []string
		for bi, bl := range block {
			t := strings.TrimSpace(bl)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if !strings.HasPrefix(t, "- ") {
				return fmVal{}, 0, fmt.Errorf("line %d: expected \"- item\" in block list", lineNo+1+bi)
			}
			s, err := parseScalar(strings.TrimPrefix(t, "- "), lineNo+1+bi)
			if err != nil {
				return fmVal{}, 0, err
			}
			if s == "" {
				return fmVal{}, 0, fmt.Errorf("line %d: empty block-list item", lineNo+1+bi)
			}
			list = append(list, s)
		}
		return fmVal{IsList: true, List: list}, consumed, nil
	}

	// Nested map of scalars/flow lists.
	m := newFmMap()
	for bi := 0; bi < len(block); bi++ {
		bl := block[bi]
		blNo := lineNo + 1 + bi
		t := strings.TrimSpace(bl)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, rest, ok := strings.Cut(bl, ":")
		if !ok {
			return fmVal{}, 0, fmt.Errorf("line %d: expected \"key: value\" in block", blNo)
		}
		key = strings.TrimSpace(key)
		if !validFmKey(key) {
			return fmVal{}, 0, fmt.Errorf("line %d: invalid key %q", blNo, key)
		}
		v, sub, err := parseValue(rest, nil, 0, blNo, true)
		if err != nil {
			return fmVal{}, 0, err
		}
		_ = sub
		if err := m.set(key, v); err != nil {
			return fmVal{}, 0, fmt.Errorf("line %d: %w", blNo, err)
		}
	}
	return fmVal{Map: m}, consumed, nil
}

// nextContent returns the index of the next non-blank, non-comment line at
// or after j, or -1.
func nextContent(lines []string, j int) int {
	for k := j; k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t != "" && !strings.HasPrefix(t, "#") {
			return k
		}
	}
	return -1
}
