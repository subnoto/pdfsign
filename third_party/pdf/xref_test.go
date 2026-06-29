package pdf

import (
	"bytes"
	"fmt"
	"testing"
)

func TestReadXrefTable(t *testing.T) {
	// Dynamically build a minimal PDF and calculate offsets
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
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[1]))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[2]))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[3]))

	buf.WriteString("trailer\n<< /Size 4 /Root 1 0 R >>\n")
	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefPos))
	buf.WriteString("%%EOF\n")

	data := buf.Bytes()
	r := bytes.NewReader(data)
	_, err := NewReader(r, int64(len(data)))
	if err != nil {
		t.Errorf("NewReader failed: %v", err)
	}
}

func TestReadXrefStream(t *testing.T) {
	var buf bytes.Buffer
	offsets := make(map[int]int)

	buf.WriteString("%PDF-1.5\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n")

	writeXrefEntry := func(w *bytes.Buffer, typ byte, off int, gen uint16) {
		w.WriteByte(typ)
		w.WriteByte(byte(off >> 24))
		w.WriteByte(byte(off >> 16))
		w.WriteByte(byte(off >> 8))
		w.WriteByte(byte(off))
		w.WriteByte(byte(gen >> 8))
		w.WriteByte(byte(gen))
	}

	var xrefBin bytes.Buffer
	writeXrefEntry(&xrefBin, 0, 0, 65535)
	writeXrefEntry(&xrefBin, 1, offsets[1], 0)
	writeXrefEntry(&xrefBin, 1, offsets[2], 0)
	writeXrefEntry(&xrefBin, 1, offsets[3], 0)

	offsets[4] = buf.Len()
	writeXrefEntry(&xrefBin, 1, offsets[4], 0)

	fmt.Fprintf(&buf, "4 0 obj\n<< /Type /XRef /Size 5 /Root 1 0 R /W [1 4 2] /Index [0 5] /Length %d >>\nstream\n", xrefBin.Len())
	buf.Write(xrefBin.Bytes())
	buf.WriteString("\nendstream\nendobj\n")

	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", offsets[4])
	buf.WriteString("%%EOF\n")

	data := buf.Bytes()
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	if r.XrefInformation.Type != "stream" {
		t.Fatalf("expected xref type stream, got %q", r.XrefInformation.Type)
	}

	xref := r.Xref()
	if len(xref) < 5 {
		t.Fatalf("expected at least 5 xref entries, got %d", len(xref))
	}
	for id, wantOff := range offsets {
		if xref[id].offset != int64(wantOff) {
			t.Errorf("xref[%d].offset = %d, want %d", id, xref[id].offset, wantOff)
		}
	}

	root := r.Trailer().Key("Root")
	if root.GetPtr().GetID() != 1 {
		t.Fatalf("Root object id = %d, want 1", root.GetPtr().GetID())
	}
	cat, err := r.GetObject(1)
	if err != nil {
		t.Fatalf("GetObject(1): %v", err)
	}
	if cat.Key("Type").Name() != "Catalog" {
		t.Errorf("catalog Type = %q, want Catalog", cat.Key("Type").Name())
	}
}
