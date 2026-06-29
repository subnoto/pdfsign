package sign

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func TestVisualSignature(t *testing.T) {
	input_file, err := os.Open("../testfiles/testfile20.pdf")
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}

	finfo, err := input_file.Stat()
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}
	size := finfo.Size()

	rdr, err := pdf.NewReader(input_file, size)
	if err != nil {
		t.Errorf("Failed to load test PDF")
		return
	}

	timezone, _ := time.LoadLocation("Europe/Tallinn")
	now := time.Date(2017, 9, 23, 14, 39, 0, 0, timezone)

	sign_data := SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:        "John Doe",
				Location:    "Somewhere",
				Reason:      "Test",
				ContactInfo: "None",
				Date:        now,
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
	}

	sign_data.objectId = uint32(rdr.XrefInformation.ItemCount) + 3

	context := SignContext{
		PDFReader: rdr,
		InputFile: input_file,
		SignData:  sign_data,
	}

	expected_visual_signature := "<<\n  /Type /Annot\n  /Subtype /Widget\n  /Rect [0 0 0 0]\n  /P 4 0 R\n  /F 132\n  /FT /Sig\n  /T (Signature 1)\n  /V 13 0 R\n>>\n"

	visual_signature, err := context.createVisualSignature(false, 1, [4]float64{0, 0, 0, 0})
	if err != nil {
		t.Errorf("%s", err.Error())
		return
	}

	if string(visual_signature) != expected_visual_signature {
		t.Errorf("Visual signature mismatch, expected\n%q\nbut got\n%q", expected_visual_signature, visual_signature)
	}
}

func pagePDFWithStreamRef() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off3 := buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /AAPL:PPK 4 0 R >>\nendobj\n")
	off4 := buf.Len()
	buf.WriteString("4 0 obj\n<< /Length 5 >>\nstream\nhello\nendstream\nendobj\n")
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", off1, off2, off3, off4)
	buf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(&buf, "%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}

func TestPageSerializationStreamValue(t *testing.T) {
	data := pagePDFWithStreamRef()
	rdr, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	context := SignContext{
		PDFReader: rdr,
	}

	pageUpdate, err := context.createIncPageUpdate(1, 99)
	if err != nil {
		t.Fatalf("createIncPageUpdate: %v", err)
	}
	out := string(pageUpdate)
	if strings.Contains(out, "@") {
		t.Fatalf("page update must not contain debug @offset syntax: %s", out)
	}
	if !strings.Contains(out, "/AAPL:PPK 4 0 R") {
		t.Fatalf("expected stream ref as indirect object, got:\n%s", out)
	}
}
