package sign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

// assertPAdESSignatureObjects checks ISO 32000 / PAdES-relevant dictionary fields
// on every Adobe.PPKLite signature object in the PDF.
func assertPAdESSignatureObjects(t *testing.T, rdr *pdf.Reader) {
	t.Helper()
	found := 0
	for _, x := range rdr.Xref() {
		v, err := rdr.GetObject(x.Ptr().GetID())
		if err != nil {
			continue
		}
		if v.Key("Filter").Name() != "Adobe.PPKLite" {
			continue
		}
		found++
		sub := v.Key("SubFilter").Name()
		switch sub {
		case "adbe.pkcs7.detached", "ETSI.RFC3161":
		default:
			t.Fatalf("signature %d: unexpected /SubFilter %q", found, sub)
		}

		if typ := v.Key("Type").Name(); typ != "" && typ != "Sig" && typ != "DocTimeStamp" {
			t.Fatalf("signature %d: unexpected /Type %q", found, typ)
		}

		br := v.Key("ByteRange")
		if br.IsNull() {
			t.Fatalf("signature %d: missing /ByteRange", found)
		}
		if br.Len() != 4 {
			t.Fatalf("signature %d: /ByteRange must have 4 entries, got %d", found, br.Len())
		}
		offset := br.Index(0).Int64()
		len1 := br.Index(1).Int64()
		offset2 := br.Index(2).Int64()
		len2 := br.Index(3).Int64()
		if len1 <= 0 || len2 <= 0 {
			t.Fatalf("signature %d: invalid /ByteRange lengths", found)
		}
		if offset2 <= offset+len1 {
			t.Fatalf("signature %d: /ByteRange second span must start after /Contents gap", found)
		}

		contents := v.Key("Contents").RawString()
		if contents == "" {
			t.Fatalf("signature %d: missing /Contents", found)
		}
		if strings.EqualFold(contents, strings.Repeat("0", len(contents))) {
			t.Fatalf("signature %d: /Contents is still placeholder zeros", found)
		}

		if sub == "adbe.pkcs7.detached" {
			ref := v.Key("Reference")
			if ref.IsNull() {
				t.Fatalf("signature %d: CMS detached signature missing /Reference", found)
			}
			hasSigRef := false
			for i := 0; i < ref.Len(); i++ {
				if ref.Index(i).Key("Type").Name() == "SigRef" {
					hasSigRef = true
					break
				}
			}
			if !hasSigRef {
				t.Fatalf("signature %d: /Reference must contain /Type /SigRef", found)
			}
		}
	}
	if found == 0 {
		t.Fatal("no Adobe.PPKLite signature objects found")
	}
}

func openPDFReader(t *testing.T, path string) *pdf.Reader {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	return rdr
}

func TestPAdESSignatureStructure(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "pades-structure-")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })

	_, err = SignFile("../testfiles/testfile20.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "Structure Test",
				Date: time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:          key,
		DigestAlgorithm: 0,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPAdESSignatureObjects(t, openPDFReader(t, tmp.Name()))
}

func TestPAdESStructureOnSuccessFixtures(t *testing.T) {
	dir := "../testfiles/success"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("testfiles/success not present")
	}
	var files []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".pdf" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			continue
		}
		files = append(files, path)
	}
	if len(files) == 0 {
		t.Skip("no non-empty signed PDFs in testfiles/success")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			assertPAdESSignatureObjects(t, openPDFReader(t, path))
		})
	}
}
