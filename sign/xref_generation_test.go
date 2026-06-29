package sign

import (
	"bytes"
	"crypto"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
	"github.com/mattetti/filebuffer"
)

func TestUpdateObjectReusesGeneration(t *testing.T) {
	input, err := os.Open("../testfiles/testfile12.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(input, info.Size())
	if err != nil {
		t.Fatal(err)
	}

	page, err := findPageByNumber(rdr.Trailer().Key("Root").Key("Pages"), 1)
	if err != nil {
		t.Fatal(err)
	}
	pageID := page.GetPtr().GetID()
	startGen := page.GetPtr().GetGen()

	ctx := &SignContext{
		PDFReader:    rdr,
		OutputBuffer: filebuffer.New(nil),
	}
	if err := ctx.updateObject(pageID, []byte("<< /Type /Page >>\n")); err != nil {
		t.Fatal(err)
	}
	if len(ctx.updatedXrefEntries) != 1 {
		t.Fatalf("updatedXrefEntries len = %d, want 1", len(ctx.updatedXrefEntries))
	}
	// Incremental updates reuse the same object id at its existing generation
	// so that references elsewhere (e.g. the /Pages /Kids array) stay valid.
	wantGen := int(startGen)
	if ctx.updatedXrefEntries[0].Generation != wantGen {
		t.Fatalf("xref generation = %d, want %d", ctx.updatedXrefEntries[0].Generation, wantGen)
	}
	out := ctx.OutputBuffer.Buff.String()
	if !strings.Contains(out, fmt.Sprintf("%d %d obj", pageID, wantGen)) {
		t.Fatalf("expected object header %d %d obj in output, got:\n%s", pageID, wantGen, out)
	}
}

func TestFetchExistingSignaturesUsesWidgetNotSignatureDict(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "fetch-existing-sigs-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = SignFile("../testfiles/testfile20.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:   "First",
				Reason: "First",
				Date:   time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          key,
		Certificate:     cert,
	})
	if err != nil {
		t.Fatal(err)
	}

	signed, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = signed.Close() }()
	finfo, err := signed.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(signed, finfo.Size())
	if err != nil {
		t.Fatal(err)
	}

	ctx := &SignContext{PDFReader: rdr}
	existing, err := ctx.fetchExistingSignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(existing) != 1 {
		t.Fatalf("existing signatures = %d, want 1", len(existing))
	}

	fields := rdr.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	if fields.Len() != 1 {
		t.Fatalf("Fields len = %d, want 1", fields.Len())
	}
	fieldPtr := fields.Index(0).GetPtr()
	if existing[0].widgetID != fieldPtr.GetID() {
		t.Fatalf("widgetID = %d, want field id %d", existing[0].widgetID, fieldPtr.GetID())
	}
	if existing[0].generation != fieldPtr.GetGen() {
		t.Fatalf("generation = %d, want %d", existing[0].generation, fieldPtr.GetGen())
	}

	widget, err := rdr.GetObject(existing[0].widgetID)
	if err != nil {
		t.Fatal(err)
	}
	sigDictPtr := widget.Key("V").GetPtr()
	if sigDictPtr.GetID() == 0 {
		t.Fatal("signature widget missing /V")
	}
	if existing[0].widgetID == sigDictPtr.GetID() {
		t.Fatal("fetchExistingSignatures returned signature dictionary id instead of widget id")
	}
}

func page1ObjectID(t *testing.T, path string) uint32 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	page, err := findPageByNumber(rdr.Trailer().Key("Root").Key("Pages"), 1)
	if err != nil {
		t.Fatal(err)
	}
	return page.GetPtr().GetID()
}

func TestLatestObjectGenerationFromSignedPDF(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	pageID := page1ObjectID(t, "../testfiles/testfile12.pdf")
	tmp, err := os.CreateTemp("", "latest-gen-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = SignFile("../testfiles/testfile12.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{Name: "S", Reason: "R", Date: time.Now().Local()},
			CertType: ApprovalSignature, DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible: true, Page: 1,
			LowerLeftX: 50, LowerLeftY: 50, UpperRightX: 250, UpperRightY: 125,
			Image: mustReadFile(t, "../testfiles/pdfsign-signature.jpg"),
		},
		DigestAlgorithm: crypto.SHA512,
		Signer: key, Certificate: cert,
	})
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}

	ctx := &SignContext{PDFReader: rdr, InputFile: f}
	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(fmt.Sprintf("\n%d 0 obj", pageID))) {
		t.Fatalf("signed PDF missing %d 0 obj header; page headers: %v", pageID, objectHeaders(raw, pageID))
	}
	if got := ctx.latestObjectGeneration(pageID); got != 0 {
		t.Fatalf("latestObjectGeneration(%d) = %d, want 0", pageID, got)
	}
}

func objectHeaders(data []byte, id uint32) []string {
	re := regexp.MustCompile(fmt.Sprintf(`\n%d \d+ obj`, id))
	return re.FindAllString(string(data), -1)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSignedPDFIncrementalXrefGeneration(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	pageID := page1ObjectID(t, "../testfiles/testfile12.pdf")
	tmp, err := os.CreateTemp("", "xref-gen-signed-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = SignFile("../testfiles/testfile12.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{Name: "S", Reason: "R", Date: time.Now().Local()},
			CertType: ApprovalSignature, DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Appearance: Appearance{
			Visible: true, Page: 1,
			LowerLeftX: 50, LowerLeftY: 50, UpperRightX: 250, UpperRightY: 125,
			Image: mustReadFile(t, "../testfiles/pdfsign-signature.jpg"),
		},
		DigestAlgorithm: crypto.SHA512,
		Signer: key, Certificate: cert,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(fmt.Sprintf("\n%d 0 obj", pageID))) {
		t.Fatalf("missing updated page object header %d 0 obj", pageID)
	}
	if !bytes.Contains(data, []byte("00000 n")) {
		t.Fatal("expected incremental xref entry with generation 0")
	}
}
