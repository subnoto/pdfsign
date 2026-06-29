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
	// TODO: Implement Xref stream test
}
