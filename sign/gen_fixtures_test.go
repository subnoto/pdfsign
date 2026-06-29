package sign

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

// Generated fixture metadata. Source PDFs are committed under testfiles/;
// regenerate with: python3 testfiles/generate/generate.py (see testfiles/generate/README.md).
var generatedFixtures = []struct {
	file       string
	version    string
	xrefStream bool
}{
	{"gen_pdf13_xref_table.pdf", "1.3", false},
	{"gen_pdf14_acroform.pdf", "1.4", false},
	{"gen_pdf15_xref_stream.pdf", "1.5", true},
	{"gen_pdf16_xref_table_3pages.pdf", "1.6", false},
	{"gen_pdf17_xref_stream_landscape.pdf", "1.7", true},
	{"gen_pdf17_nested_page_tree.pdf", "1.7", true},
	{"gen_pdf20_xref_stream.pdf", "2.0", true},
}

func TestGeneratedFixturesOpen(t *testing.T) {
	for _, fx := range generatedFixtures {
		t.Run(fx.file, func(t *testing.T) {
			path := filepath.Join("../testfiles", fx.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if !strings.HasPrefix(string(data), "%PDF-"+fx.version) {
				t.Fatalf("expected PDF version %s, header: %q", fx.version, string(data[:min(16, len(data))]))
			}
			s := string(data)
			hasStream := strings.Contains(s, "/Type /XRef")
			hasTable := strings.Contains(s, "\nxref\n")
			if fx.xrefStream && !hasStream {
				t.Fatal("expected cross-reference stream")
			}
			if !fx.xrefStream && !hasTable {
				t.Fatal("expected classic xref table")
			}

			rdr, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatalf("pdf.NewReader: %v", err)
			}
			root := rdr.Trailer().Key("Root")
			if root.IsNull() {
				t.Fatal("missing /Root")
			}
			pages := root.Key("Pages")
			if pages.IsNull() {
				t.Fatal("missing /Pages")
			}
			if pages.Key("Count").Int64() < 1 {
				t.Fatal("expected at least one page")
			}
		})
	}
}

func TestGeneratedFixturesSign(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	for _, fx := range generatedFixtures {
		t.Run(fx.file, func(t *testing.T) {
			inputPath := filepath.Join("../testfiles", fx.file)
			tmp, err := os.CreateTemp("", "gen_sign_")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(tmp.Name()) }()

			_, err = SignFile(inputPath, tmp.Name(), SignData{
				Signature: SignDataSignature{
					Info: SignDataSignatureInfo{
						Name: "Generated Fixture",
						Date: time.Now(),
					},
					CertType: ApprovalSignature,
				},
				Signer:      key,
				Certificate: cert,
			})
			if err != nil {
				t.Fatalf("SignFile: %v", err)
			}
			verifySignedFile(t, tmp, fx.file)
		})
	}
}

func TestGeneratedAcroFormInitials(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "gen_acroform_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = SignFile("../testfiles/gen_pdf14_acroform.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "Test User",
				Date: time.Now(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:      key,
		Certificate: cert,
		Appearance: Appearance{
			SignerUID: "test",
		},
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	sf, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sf.Close() }()
	sfi, _ := sf.Stat()
	rdr, err := pdf.NewReader(sf, sfi.Size())
	if err != nil {
		t.Fatal(err)
	}
	fields := rdr.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	found := false
	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		if strings.Contains(field.Key("T").RawString(), "initials_page_1_signer_test") {
			if strings.Contains(field.Key("V").RawString(), "TU") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected initials TU on generated acroform field")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
