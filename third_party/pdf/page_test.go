package pdf

import (
	"bytes"
	"fmt"
	"sort"
	"testing"
)

func TestReaderPage(t *testing.T) {
	data := minimalCatalogPagePDF()
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	if got := r.NumPage(); got != 1 {
		t.Fatalf("NumPage() = %d, want 1", got)
	}

	p := r.Page(1)
	if p.V.IsNull() {
		t.Fatal("Page(1) returned null page")
	}
	if p.V.Key("Type").Name() != "Page" {
		t.Fatalf("Page(1) Type = %q, want Page", p.V.Key("Type").Name())
	}
	mb := p.V.Key("MediaBox")
	if mb.Kind() != Array || mb.Len() != 4 {
		t.Fatalf("Page(1) MediaBox = %v, want 4-element array", mb)
	}

	missing := r.Page(2)
	if !missing.V.IsNull() {
		t.Fatalf("Page(2) should be null, got %#v", missing.V)
	}
}

func minimalCatalogPagePDF() []byte {
	var buf bytes.Buffer
	offsets := make(map[int]int)

	buf.WriteString("%PDF-1.4\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n")

	xrefPos := buf.Len()
	buf.WriteString("xref\n0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[1])
	fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[2])
	fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[3])

	buf.WriteString("trailer\n<< /Size 4 /Root 1 0 R >>\n")
	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefPos)
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func TestPageInheritance(t *testing.T) {
	pages := Object{Kind: Dict, DictVal: make(map[string]Object)}
	page := Object{Kind: Dict, DictVal: make(map[string]Object)}

	pages.DictVal["MediaBox"] = Object{Kind: Array, ArrayVal: []Object{{Kind: Integer, Int64Val: 0}, {Kind: Integer, Int64Val: 0}, {Kind: Integer, Int64Val: 612}, {Kind: Integer, Int64Val: 792}}}
	page.DictVal["Parent"] = Object{Kind: Dict, DictVal: pages.DictVal} // simplified for test

	p := Page{V: Value{obj: page}}
	mb := p.findInherited("MediaBox")
	if mb.Kind() != Array || mb.Len() != 4 {
		t.Errorf("MediaBox inheritance failed: got %v", mb)
	}
}

func TestFontMethods(t *testing.T) {
	v := Object{Kind: Dict, DictVal: make(map[string]Object)}
	v.DictVal["BaseFont"] = Object{Kind: Name, NameVal: "Helvetica"}
	v.DictVal["FirstChar"] = Object{Kind: Integer, Int64Val: 32}
	v.DictVal["LastChar"] = Object{Kind: Integer, Int64Val: 126}
	v.DictVal["Widths"] = Object{Kind: Array, ArrayVal: []Object{}}

	f := Font{V: Value{obj: v}}
	if f.BaseFont() != "Helvetica" {
		t.Errorf("BaseFont mismatch: %q", f.BaseFont())
	}
	if f.FirstChar() != 32 {
		t.Errorf("FirstChar mismatch: %d", f.FirstChar())
	}
	if f.LastChar() != 126 {
		t.Errorf("LastChar mismatch: %d", f.LastChar())
	}
}

func TestPageResources(t *testing.T) {
	res := Object{Kind: Dict, DictVal: make(map[string]Object)}
	fonts := Object{Kind: Dict, DictVal: make(map[string]Object)}
	f1 := Object{Kind: Dict, DictVal: map[string]Object{"BaseFont": {Kind: Name, NameVal: "F1"}}}
	fonts.DictVal["F1"] = f1
	res.DictVal["Font"] = fonts

	page := Object{Kind: Dict, DictVal: make(map[string]Object)}
	page.DictVal["Resources"] = res

	p := Page{V: Value{obj: page}}
	if p.Resources().Kind() != Dict {
		t.Error("Resources() failed")
	}

	fNames := p.Fonts()
	if len(fNames) != 1 || fNames[0] != "F1" {
		t.Errorf("Fonts() failed: got %v", fNames)
	}

	font := p.Font("F1")
	if font.BaseFont() != "F1" {
		t.Errorf("Font(F1) failed: got %q", font.BaseFont())
	}
}

func TestOutline(t *testing.T) {
	// Root -> Outlines -> First -> Next
	//             |
	//           Title

	child2 := Object{Kind: Dict, DictVal: map[string]Object{
		"Title": {Kind: String, StringVal: "Chapter 2"},
	}}
	child1 := Object{Kind: Dict, DictVal: map[string]Object{
		"Title": {Kind: String, StringVal: "Chapter 1"},
		"Next":  child2,
	}}
	outlines := Object{Kind: Dict, DictVal: map[string]Object{
		"First": child1,
	}}
	root := Object{Kind: Dict, DictVal: map[string]Object{
		"Outlines": outlines,
	}}

	r := &Reader{}
	r.trailer = Object{Kind: Dict, DictVal: map[string]Object{"Root": root}}

	out := r.Outline()
	if len(out.Child) != 2 {
		t.Fatalf("expected 2 top-level outline entries, got %d", len(out.Child))
	}
	if out.Child[0].Title != "Chapter 1" {
		t.Errorf("expected Chapter 1, got %q", out.Child[0].Title)
	}
	if out.Child[1].Title != "Chapter 2" {
		t.Errorf("expected Chapter 2, got %q", out.Child[1].Title)
	}
}

func TestTextHorizontalSort(t *testing.T) {
	th := TextHorizontal{
		{S: "B", X: 20, Y: 10},
		{S: "A", X: 10, Y: 10},
		{S: "C", X: 10, Y: 20},
	}
	sort.Sort(th)
	// Order: C (10, 20), A (10, 10), B (20, 10)
	if th[0].S != "C" || th[1].S != "A" || th[2].S != "B" {
		t.Errorf("Horizontal sort failed: got %v", th)
	}
	// Coverage for Swap
	th.Swap(0, 1)
	if th[0].S != "A" {
		t.Errorf("Swap failed")
	}
}

func TestTextVerticalSort(t *testing.T) {
	tv := TextVertical{
		{S: "B", X: 10, Y: 10},
		{S: "A", X: 10, Y: 20},
		{S: "C", X: 20, Y: 10},
	}
	sort.Sort(tv)
	if tv[0].S != "A" || tv[1].S != "B" || tv[2].S != "C" {
		t.Errorf("Vertical sort failed: %v", tv)
	}
	// Coverage for Swap
	tv.Swap(0, 1)
	if tv[0].S != "B" {
		t.Errorf("Swap failed")
	}
}

func TestPageSortingLen(t *testing.T) {
	tv := TextVertical{{S: "A"}}
	if tv.Len() != 1 {
		t.Error("Vertical Len failure")
	}
	th := TextHorizontal{{S: "A"}}
	if th.Len() != 1 {
		t.Error("Horizontal Len failure")
	}
}

func TestPageContent(t *testing.T) {
	// Mock font with Widths
	fontDict := Object{Kind: Dict, DictVal: map[string]Object{
		"Type":      {Kind: Name, NameVal: "Font"},
		"Subtype":   {Kind: Name, NameVal: "Type1"},
		"BaseFont":  {Kind: Name, NameVal: "Helvetica"},
		"FirstChar": {Kind: Integer, Int64Val: 65},
		"LastChar":  {Kind: Integer, Int64Val: 66},
		"Widths":    {Kind: Array, ArrayVal: []Object{{Kind: Integer, Int64Val: 600}, {Kind: Integer, Int64Val: 600}}},
	}}

	res := Object{Kind: Dict, DictVal: map[string]Object{
		"Font": {Kind: Dict, DictVal: map[string]Object{"F1": fontDict}},
	}}

	data := []byte("BT /F1 12 Tf 10 20 Td (AB) Tj ET")
	r := &Reader{f: bytes.NewReader(data)}

	page := Object{Kind: Dict, DictVal: map[string]Object{
		"Resources": res,
		"Contents": {
			Kind:         Stream,
			DictVal:      map[string]Object{"Length": {Kind: Integer, Int64Val: int64(len(data))}},
			StreamOffset: 0,
		},
	}}

	p := Page{V: Value{r: r, obj: page}}
	content := p.Content()

	if len(content.Text) != 2 {
		t.Errorf("expected 2 characters, got %d", len(content.Text))
	}

	// 'A' is 65.
	if content.Text[0].S != "A" {
		t.Errorf("expected A, got %q", content.Text[0].S)
	}
}

func TestReadCmap(t *testing.T) {
	// Simple CMap: 16-bit to 16-bit mapping
	data := []byte(`
1 begincodespacerange
  <0041> <0041>
endcodespacerange
1 beginbfrange
  <0041> <0041> <0042>
endbfrange
`) // A -> B
	r := &Reader{f: bytes.NewReader(data)}
	strm := Value{
		r: r,
		obj: Object{
			Kind:         Stream,
			DictVal:      map[string]Object{"Length": {Kind: Integer, Int64Val: int64(len(data))}},
			StreamOffset: 0,
		},
	}

	cmap := readCmap(strm)
	if cmap == nil {
		t.Fatal("readCmap returned nil")
	}

	decoded := cmap.Decode("\x00\x41")
	if decoded != "B" {
		t.Errorf("expected B, got %q", decoded)
	}
}

func TestFontWidth(t *testing.T) {
	// Simple font with Widths
	v := Object{Kind: Dict, DictVal: map[string]Object{
		"BaseFont":  {Kind: Name, NameVal: "Helvetica"},
		"FirstChar": {Kind: Integer, Int64Val: 32},
		"LastChar":  {Kind: Integer, Int64Val: 33},
		"Widths": {Kind: Array, ArrayVal: []Object{
			{Kind: Integer, Int64Val: 278},
			{Kind: Integer, Int64Val: 278},
		}},
	}}
	f := Font{V: Value{obj: v}}

	if f.Width(32) != 278 {
		t.Errorf("expected 278, got %f", f.Width(32))
	}
	if f.Width(34) != 0 {
		t.Errorf("expected 0 for out of range, got %f", f.Width(34))
	}
}

func TestEncoders(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
	}{
		{"WinAnsi", "WinAnsiEncoding"},
		{"MacRoman", "MacRomanEncoding"},
		{"Identity-H", "Identity-H"},
		{"Unknown", "UnknownEncoding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Font{V: Value{obj: Object{Kind: Dict, DictVal: map[string]Object{
				"Encoding": {Kind: Name, NameVal: tt.encoding},
			}}}}
			enc := f.Encoder()
			if enc == nil {
				t.Errorf("%s: Encoder() returned nil", tt.name)
			}
			// Test decode (nopEncoder or byteEncoder)
			_ = enc.Decode("A")
		})
	}
}

func TestFontWidths(t *testing.T) {
	v := Object{Kind: Dict, DictVal: map[string]Object{
		"Widths": {Kind: Array, ArrayVal: []Object{
			{Kind: Integer, Int64Val: 100},
			{Kind: Integer, Int64Val: 200},
		}},
	}}
	f := Font{V: Value{obj: v}}
	widths := f.Widths()
	if len(widths) != 2 || widths[0] != 100 || widths[1] != 200 {
		t.Errorf("Widths() mismatch: %v", widths)
	}
}

func TestPageContentOperators(t *testing.T) {
	// BT, ET, Tf, Tj already tested in TestPageContent.
	// Test: cm, gs, re, q, Q, T*, Tc, TD, Td, TJ, TL, Tm, Tr, Ts, Tw, g, rg, RG
	data := []byte(`
q
1 0 0 1 10 20 cm
BT
/F1 12 Tf
10 Tc
5 Tw
15 TL
2 Ts
1 Tr
[ (A) 100 (B) ] TJ
T*
10 10 Td
20 20 TD
(Text) Tj
ET
10 20 30 40 re f
0.5 g
1 0 0 rg
0 1 0 RG
Q
`)
	r := &Reader{f: bytes.NewReader(data)}

	// Mock Resource for gs
	res := Object{Kind: Dict, DictVal: map[string]Object{
		"ExtGState": {Kind: Dict, DictVal: map[string]Object{
			"GS1": {Kind: Dict, DictVal: map[string]Object{}},
		}},
	}}

	page := Object{Kind: Dict, DictVal: map[string]Object{
		"Resources": res,
		"Contents": {
			Kind:         Stream,
			DictVal:      map[string]Object{"Length": {Kind: Integer, Int64Val: int64(len(data))}},
			StreamOffset: 0,
		},
	}}
	p := Page{V: Value{r: r, obj: page}}
	content := p.Content()

	if len(content.Rect) != 1 {
		t.Errorf("expected 1 rect, got %d", len(content.Rect))
	}
	// Total text elements: 'A', 'B' (from TJ), 'Text' (from Tj)
	// Tj(Text) -> T, e, x, t
	if len(content.Text) < 3 {
		t.Errorf("expected at least 3 text elements, got %d", len(content.Text))
	}
}
