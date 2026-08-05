package webui

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// The cascade-layer relationship in the SHIPPED stylesheet (P3-UI-7 R4;
// CONVENTIONS §52-B).
//
// Why this test exists, and why it lives in Go rather than in the web suite:
// jsdom computes no cascade, so every vitest in the tree passes whether or not
// an owned rule is suppressing a utility. §52-B is the proven cost of that
// blind spot — an unlayered `input,select,button` rule beat every
// `@layer utilities` rule regardless of specificity and silently suppressed the
// kit Button's palette across eleven surfaces for two packets. Only the BUILT
// sheet carries the evidence, and the Go suite is the one that runs after
// `npm run build`. A vitest reading dist/ would read the PREVIOUS build, since
// the battery order is typecheck → test → build.
//
// It asserts a RELATIONSHIP, never a byte offset. §52 drain D4a recorded why:
// the offsets first written into that section were read off an intermediate
// build and did not reproduce, which is the one thing a byte offset must do.
// What must hold is ordinal — the owned rules sit in a layer the sheet's own
// layer order places BEFORE `utilities` — and that survives any re-minification.

// ownedMarkers are rules this stylesheet owns, chosen because they are the two
// §52-B named as unlayered and because each is a bare ELEMENT selector: a
// utility can never out-specify one, so their layer position is the whole
// question. They are matched on the minified form Lightning CSS emits.
var ownedMarkers = []string{
	// The global form chrome. This is the rule that suppressed the kit Button.
	"input,select,button{",
	// The preflight-compatibility type rhythm.
	"p,dl,dd,",
}

// utilityMarker is a utility the kit demonstrably compiles (ui/Toast.tsx's
// Title carries `m-0`). Asserting the utilities layer really contains it stops
// this test from passing over a sheet where Tailwind emitted nothing.
const utilityMarker = ".m-0{"

// layerSpan is one top-level `@layer NAME { … }` block in the sheet.
type layerSpan struct {
	name       string
	open, shut int // byte offsets of the block's `{` and its matching `}`
}

// readShippedCSS returns the single stylesheet Vite emitted into the embedded
// build. It is the real shipped bytes: no re-render, no re-parse, no fixture.
func readShippedCSS(t *testing.T) string {
	t.Helper()
	assets := Assets()
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		t.Fatalf("read assets/: %v", err)
	}
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".css") {
			found = append(found, e.Name())
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one stylesheet in the build, got %v — the SPA is un-split by design", found)
	}
	b, err := fs.ReadFile(assets, path.Join("assets", found[0]))
	if err != nil {
		t.Fatalf("read %s: %v", found[0], err)
	}
	return string(b)
}

// scanLayers walks the sheet once and returns the sheet's own layer ORDER
// (names in first-appearance order, which is what establishes precedence when
// no explicit `@layer a, b;` statement is emitted) together with every
// top-level layer BLOCK.
//
// Two lexical hazards are handled, and BOTH were found in the real sheet rather
// than imagined:
//
//   - Quoted strings are skipped, so a brace inside `content:"{"` cannot desync
//     the depth count.
//   - A BACKSLASH ESCAPE outside a string swallows the next character. Tailwind
//     escapes arbitrary characters in generated class names, and the shipped
//     sheet really carries a selector of the form `.\"\]:before` — reading that
//     `\"` as a string opener swallowed 20 kB of the sheet and made the
//     `utilities` layer vanish from the scan. An instrument that silently loses
//     the layer it is measuring reports "no defect" for the wrong reason.
func scanLayers(css string) (order []string, blocks []layerSpan) {
	seen := map[string]bool{}
	note := func(names string) {
		for _, n := range strings.Split(names, ",") {
			n = strings.TrimSpace(n)
			if n != "" && !seen[n] {
				seen[n] = true
				order = append(order, n)
			}
		}
	}

	depth := 0
	var open []int     // byte offset of each open brace, by depth
	var heads []string // the text preceding each open brace, by depth
	head := 0          // start of the current at-rule prelude

	for i := 0; i < len(css); i++ {
		switch c := css[i]; c {
		case '\\':
			i++ // an escape outside a string: the next character is literal
		case '"', '\'':
			for i++; i < len(css) && css[i] != c; i++ {
				if css[i] == '\\' {
					i++
				}
			}
		case '{':
			open = append(open, i)
			heads = append(heads, strings.TrimSpace(css[head:i]))
			depth++
			head = i + 1
		case '}':
			if depth > 0 {
				depth--
				prelude := heads[depth]
				if depth == 0 && strings.HasPrefix(prelude, "@layer") {
					name := strings.TrimSpace(strings.TrimPrefix(prelude, "@layer"))
					note(name)
					blocks = append(blocks, layerSpan{name: name, open: open[depth], shut: i})
				}
				open, heads = open[:depth], heads[:depth]
			}
			head = i + 1
		case ';':
			if depth == 0 {
				if prelude := strings.TrimSpace(css[head:i]); strings.HasPrefix(prelude, "@layer") {
					note(strings.TrimSpace(strings.TrimPrefix(prelude, "@layer")))
				}
			}
			head = i + 1
		}
	}
	return order, blocks
}

// enclosingLayer names the top-level layer block holding off, or "" when the
// rule is unlayered — which is the defect this test exists to catch, because an
// unlayered rule outranks every layered one before specificity is consulted.
func enclosingLayer(blocks []layerSpan, off int) string {
	for _, b := range blocks {
		if off > b.open && off < b.shut {
			return b.name
		}
	}
	return ""
}

func orderOf(order []string, name string) int {
	for i, n := range order {
		if n == name {
			return i
		}
	}
	return -1
}

// TestOwnedRulesLoseToUtilitiesInTheShippedSheet is the standing guard on the
// §52-B hazard class. It fails against any build in which an owned element rule
// sits outside every cascade layer.
func TestOwnedRulesLoseToUtilitiesInTheShippedSheet(t *testing.T) {
	if !Built() {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §41, web-assets): no SPA build embedded — run `cd web && npm run build`")
	}
	css := readShippedCSS(t)
	order, blocks := scanLayers(css)

	utilities := orderOf(order, "utilities")
	if utilities < 0 {
		t.Fatalf("the sheet declares no `utilities` layer; layer order is %v", order)
	}

	// The utilities layer is real and non-empty. Without this the ordinal
	// comparison below could hold over a sheet that compiled no utility at all.
	var utilBlock *layerSpan
	for i := range blocks {
		if blocks[i].name == "utilities" {
			utilBlock = &blocks[i]
			break
		}
	}
	if utilBlock == nil {
		t.Fatal("`utilities` is named in the layer order but has no block in the sheet")
	}
	if !strings.Contains(css[utilBlock.open:utilBlock.shut], utilityMarker) {
		t.Errorf("the utilities layer does not carry %q — the kit's utilities did not compile, so this test would prove nothing", utilityMarker)
	}

	for _, marker := range ownedMarkers {
		at := strings.Index(css, marker)
		if at < 0 {
			t.Errorf("owned rule %q is not in the shipped sheet — this test cannot pass by the rule disappearing", marker)
			continue
		}
		if strings.Index(css[at+1:], marker) >= 0 {
			t.Errorf("owned rule %q appears more than once; the marker no longer identifies one rule", marker)
			continue
		}
		layer := enclosingLayer(blocks, at)
		if layer == "" {
			t.Errorf("owned rule %q is UNLAYERED: it beats every @layer utilities rule regardless of specificity, "+
				"which is the §52-B suppression (kit Button palette, width:100%%). Wrap the owned sheet in a "+
				"cascade layer ordered before `utilities`.", marker)
			continue
		}
		if got := orderOf(order, layer); got < 0 || got >= utilities {
			t.Errorf("owned rule %q sits in layer %q at order %d, but `utilities` is at %d — an owned rule must be "+
				"in an EARLIER layer for kit utilities to win as authored. Layer order: %v",
				marker, layer, got, utilities, order)
		}
	}
}

// TestTheLayerScannerCanFail plants both defect shapes the scanner must catch,
// so a green run above means the instrument works rather than that it saw
// nothing. A detector that has never fired is a detector nobody has checked.
func TestTheLayerScannerCanFail(t *testing.T) {
	// An unlayered owned rule below a utilities layer: the pre-fix shape.
	broken := `@layer theme{:root{--x:1}}@layer utilities{.m-0{margin:0}}input,select,button{width:100%}`
	order, blocks := scanLayers(broken)
	if got := enclosingLayer(blocks, strings.Index(broken, "input,select,button{")); got != "" {
		t.Errorf("the scanner read an unlayered rule as being in layer %q", got)
	}
	if orderOf(order, "utilities") < 0 {
		t.Error("the scanner missed the utilities layer")
	}

	// A layered owned rule, but in a layer ordered AFTER utilities.
	late := `@layer utilities{.m-0{margin:0}}@layer late{input,select,button{width:100%}}`
	order, blocks = scanLayers(late)
	if got := enclosingLayer(blocks, strings.Index(late, "input,select,button{")); got != "late" {
		t.Errorf("enclosing layer = %q, want late", got)
	}
	if orderOf(order, "late") <= orderOf(order, "utilities") {
		t.Error("the scanner ordered a later layer before an earlier one")
	}

	// The fixed shape: an earlier layer wins the ordinal comparison.
	fixed := `@layer base{input,select,button{width:100%}}@layer utilities{.m-0{margin:0}}`
	order, blocks = scanLayers(fixed)
	if got := enclosingLayer(blocks, strings.Index(fixed, "input,select,button{")); got != "base" {
		t.Errorf("enclosing layer = %q, want base", got)
	}
	if orderOf(order, "base") >= orderOf(order, "utilities") {
		t.Error("base must order before utilities")
	}

	// A brace inside a quoted value must not desync the depth count.
	quoted := `@layer base{.q:before{content:"{"}}@layer utilities{.m-0{margin:0}}`
	order, blocks = scanLayers(quoted)
	if got := enclosingLayer(blocks, strings.Index(quoted, ".q:before")); got != "base" {
		t.Errorf("a quoted brace desynced the scanner: enclosing layer = %q, want base", got)
	}
	if orderOf(order, "utilities") != 1 {
		t.Errorf("layer order = %v, want base then utilities", order)
	}

	// An ESCAPED QUOTE in a class-name selector must not open a string. This is
	// the shape that really appears in the shipped sheet (Tailwind escapes the
	// character in a generated class name); reading it as a string opener made
	// the whole utilities layer disappear from the scan.
	escaped := `@layer utilities{.\"\]:before{--tw-content:"\""}}@layer late{input,select,button{width:100%}}`
	order, blocks = scanLayers(escaped)
	if orderOf(order, "utilities") != 0 || orderOf(order, "late") != 1 {
		t.Errorf("an escaped quote in a selector desynced the scanner: layer order = %v, want [utilities late]", order)
	}
	if got := enclosingLayer(blocks, strings.Index(escaped, "input,select,button{")); got != "late" {
		t.Errorf("an escaped quote in a selector desynced the scanner: enclosing layer = %q, want late", got)
	}

	// A bare `@layer a, b;` order statement establishes precedence with no block.
	stated := `@layer base, utilities;@layer utilities{.m-0{margin:0}}`
	order, _ = scanLayers(stated)
	if orderOf(order, "base") != 0 || orderOf(order, "utilities") != 1 {
		t.Errorf("layer order = %v, want [base utilities] from the order statement", order)
	}
}
