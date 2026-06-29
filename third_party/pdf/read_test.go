package pdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestReadObject(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKind Kind
	}{
		{"Dictionary", "<< /Key1 (Val1) /Key2 123 >> ", Dict},
		{"Array", "[ 1 2 (3) /Name ] ", Array},
		{"Nested", "<< /Arr [ 1 << /K /V >> ] >> ", Dict},
		{"Indirect", "10 0 R ", Indirect},
		{"HexString", "<414243> ", String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBuffer(io.NewSectionReader(bytes.NewReader([]byte(tt.input)), 0, int64(len(tt.input))), 0, 0)
			obj := b.readObject()
			if obj.Kind != tt.wantKind {
				t.Errorf("%s: readObject().Kind = %v, want %v", tt.name, obj.Kind, tt.wantKind)
			}
		})
	}
}

func TestReader(t *testing.T) {
	// Use testfile12.pdf from root testfiles
	file := "../../testfiles/testfile12.pdf"
	f, err := os.Open(file)
	if err != nil {
		t.Skip("testfile12.pdf not found, skipping integration test")
		return
	}
	defer f.Close()

	fi, _ := f.Stat()
	r, err := NewReader(f, fi.Size())
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	if r.NumPage() == 0 {
		t.Error("NumPage() returned 0")
	}

	// Try to resolve an object
	found := false
	for id, x := range r.xref {
		if x.offset > 0 {
			obj, err := r.GetObject(uint32(id))
			if err != nil {
				t.Errorf("GetObject(%d) failed: %v", id, err)
			} else if obj.Kind() == Null {
				t.Errorf("GetObject(%d) returned Null", id)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("No objects found in xref")
	}
}

func TestReadDict(t *testing.T) {
	input := "<< /Type /Catalog /Pages 2 0 R /Empty () >>"
	b := newBuffer(io.NewSectionReader(bytes.NewReader([]byte(input)), 0, int64(len(input))), 0, 0)
	// skip '<<'
	b.readToken()
	obj := b.readDict()

	if obj.Kind != Dict {
		t.Fatalf("Expected Dict, got %v", obj.Kind)
	}

	if obj.DictVal["Type"].NameVal != "Catalog" {
		t.Errorf("Type mismatch: %q", obj.DictVal["Type"].NameVal)
	}

	if obj.DictVal["Pages"].Kind != Indirect {
		t.Errorf("Pages should be Indirect, got %v", obj.DictVal["Pages"].Kind)
	}

	if obj.DictVal["Pages"].PtrVal.id != 2 {
		t.Errorf("Pages ID mismatch: %d", obj.DictVal["Pages"].PtrVal.id)
	}
}

func TestReadArray(t *testing.T) {
	input := "[ 1 2.5 (string) /Name [ 3 ] ]"
	b := newBuffer(io.NewSectionReader(bytes.NewReader([]byte(input)), 0, int64(len(input))), 0, 0)
	// skip '['
	b.readToken()
	obj := b.readArray()

	if obj.Kind != Array {
		t.Fatalf("Expected Array, got %v", obj.Kind)
	}

	if len(obj.ArrayVal) != 5 {
		t.Errorf("Length mismatch: %d", len(obj.ArrayVal))
	}

	if obj.ArrayVal[0].Int64Val != 1 {
		t.Errorf("Index 0 mismatch: %d", obj.ArrayVal[0].Int64Val)
	}

	if obj.ArrayVal[1].Float64Val != 2.5 {
		t.Errorf("Index 1 mismatch: %f", obj.ArrayVal[1].Float64Val)
	}
}

func TestOpen(t *testing.T) {
	// Root testfiles
	file := "../../testfiles/testfile12.pdf"
	r, err := Open(file)
	if err != nil {
		t.Skipf("Open failed: %v", err)
	}
	defer r.Close()

	if r.NumPage() == 0 {
		t.Error("Open() returned reader with 0 pages")
	}
}

func TestReaderUtilities(t *testing.T) {
	r := &Reader{}
	r.trailer = Object{Kind: Dict, DictVal: map[string]Object{"Size": {Kind: Integer, Int64Val: 10}}}

	if r.Trailer().Kind() != Dict {
		t.Error("Trailer() failed")
	}

	if len(r.Xref()) != 0 {
		t.Error("Xref() should be empty for new reader")
	}

	dict := GetDict()
	if dict.Kind != Dict {
		t.Error("GetDict() failed")
	}
}

func TestNewReaderEncryptedV5(t *testing.T) {
	uHex := "8a35e0ef6b995a3af7a084c7b39f3f9aa96f4ce6b961d27d5ee084a779b93ec331323334353637383837363534333231"
	ueHex := "fdf2ebcf67bd7c6f527008513dd4c01c4d5a3db53b16f3713ab07e58e67026e9"

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	off1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count 0 /Kids [] >>\nendobj\n")
	off3 := buf.Len()
	buf.WriteString(fmt.Sprintf("3 0 obj\n<< /Filter /Standard /V 5 /R 5 /O <%s> /U <%s> /OE <%s> /UE <%s> >>\nendobj\n", uHex, uHex, ueHex, ueHex))
	xrefPos := buf.Len()
	buf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off1))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off2))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off3))
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R /Encrypt 3 0 R /ID [<%s><%s>] >>\n", "00112233445566778899AABBCCDDEEFF", "00112233445566778899AABBCCDDEEFF"))
	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefPos))
	buf.WriteString("%%EOF\n")

	data := buf.Bytes()
	r, err := NewReaderEncrypted(bytes.NewReader(data), int64(len(data)), func() string { return "user" })
	if err != nil {
		t.Fatalf("NewReaderEncrypted V5 failed: %v", err)
	}

	if r.encVersion != 5 {
		t.Errorf("expected encVersion 5, got %d", r.encVersion)
	}
}

func TestReader_Errorf(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("errorf did not panic")
		}
	}()
	r := &Reader{}
	r.errorf("test error")
}

func TestReaderXrefInformation_PrintDebug(t *testing.T) {
	info := &ReaderXrefInformation{
		Type: "test",
	}
	info.PrintDebug() // Just for coverage
}

func TestApplyFilter_Error(t *testing.T) {
	_, err := applyFilter(bytes.NewReader(nil), "UnknownFilter", Value{})
	if err == nil {
		t.Error("expected error for unknown filter")
	}
}

func TestNewReaderEncryptedV4(t *testing.T) {
	// AES-128 (V=4)
	data := "%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Count 0 /Kids [] >>\nendobj\n3 0 obj\n<< /Filter /Standard /V 4 /R 4 /O (owner) /U (user) /P -4 /CF << /StdCF << /CFM /AESV2 >> >> /StmF /StdCF /StrF /StdCF >>\nendobj\ntrailer\n<< /Size 4 /Root 1 0 R /Encrypt 3 0 R /ID [ (<11223344>) (<11223344>) ] >>\nstartxref\n10\n%%EOF"
	_, _ = NewReaderEncrypted(bytes.NewReader([]byte(data)), int64(len(data)), func() string { return "user" })
}

func TestCryptKeyTruncationInReadPackage(t *testing.T) {
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	ptr := objptr{id: 42, gen: 0}
	if len(cryptKey(key, false, ptr)) != 10 {
		t.Fatal("expected 10-byte object key for 40-bit RC4 document key")
	}
}

func TestSignatureContentsNotDecrypted(t *testing.T) {
	path := "../../testfiles/testfile_encrypted_signed.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture missing (%v); run sign tests to generate testfile_encrypted_signed.pdf", err)
	}

	idx := strings.LastIndex(string(data), "/Contents<")
	if idx < 0 {
		t.Fatal("fixture missing /Contents in signed encrypted PDF")
	}
	lt := idx + len("/Contents<")
	gt := bytes.IndexByte(data[lt:], '>')
	if gt < 0 {
		t.Fatal("malformed Contents in fixture")
	}
	wantHex := strings.ToLower(string(data[lt : lt+gt]))
	if len(wantHex) < 4 || wantHex[:2] != "30" {
		t.Fatalf("fixture Contents should be PKCS#7 DER hex, got prefix %q", wantHex[:min(8, len(wantHex))])
	}

	r, err := NewReaderEncrypted(bytes.NewReader(data), int64(len(data)), func() string { return "" })
	if err != nil {
		t.Fatalf("NewReaderEncrypted: %v", err)
	}

	var got []byte
	for id := range r.Xref() {
		if id == 0 {
			continue
		}
		val, err := r.GetObject(uint32(id))
		if err != nil {
			continue
		}
		if val.Key("Filter").Name() != "Adobe.PPKLite" {
			continue
		}
		got = []byte(val.Key("Contents").RawString())
		break
	}
	if len(got) == 0 {
		t.Fatal("no signature dictionary found in encrypted signed fixture")
	}
	if got[0] != 0x30 {
		t.Fatalf("Contents must not be decrypted (expected DER 0x30 prefix, got %x)", got[:min(8, len(got))])
	}
}
