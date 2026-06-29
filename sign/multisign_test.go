package sign

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func approvalSignData(t *testing.T, reason string, tsa *TSA) SignData {
	t.Helper()
	cert, key := loadCertificateAndKey(t)
	sd := SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:   "Multi-sign test",
				Reason: reason,
				Date:   time.Now().Local(),
			},
			CertType:   ApprovalSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		DigestAlgorithm: crypto.SHA512,
		Signer:          key,
		Certificate:     cert,
	}
	if tsa != nil {
		sd.TSA = *tsa
	}
	return sd
}

func signToTemp(t *testing.T, inputPath, prefix string, sd SignData) *os.File {
	t.Helper()
	out, err := os.CreateTemp("", prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(out.Name()) })
	if _, err := SignFile(inputPath, out.Name(), sd); err != nil {
		t.Fatalf("SignFile(%q): %v", inputPath, err)
	}
	if _, err := out.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertMultiSignPDF(t *testing.T, f *os.File, minSignatures int) {
	t.Helper()
	verifyAllSignaturesValid(t, f, minSignatures)
	assertPAdESSignatureObjects(t, openPDFReader(t, f.Name()))
}

func TestMultiSignThreeApprovals(t *testing.T) {
	input := "../testfiles/testfile20.pdf"
	for i := 1; i <= 3; i++ {
		out := signToTemp(t, input, fmt.Sprintf("multisign_%d_", i), approvalSignData(t, fmt.Sprintf("Approval %d", i), nil))
		assertMultiSignPDF(t, out, i)
		input = out.Name()
	}
}

func TestMultiSignVisibleTwice(t *testing.T) {
	image, err := os.ReadFile("../testfiles/pdfsign-signature.jpg")
	if err != nil {
		t.Fatal(err)
	}
	input := "../testfiles/testfile12.pdf"
	for i := 1; i <= 2; i++ {
		sd := approvalSignData(t, fmt.Sprintf("Visible %d", i), nil)
		sd.Appearance = Appearance{
			Visible:     true,
			Page:        1,
			LowerLeftX:  50 + float64((i-1)*250),
			LowerLeftY:  50,
			UpperRightX: 250 + float64((i-1)*250),
			UpperRightY: 125,
			Image:       image,
		}
		out := signToTemp(t, input, fmt.Sprintf("multisign_vis_%d_", i), sd)
		assertMultiSignPDF(t, out, i)
		input = out.Name()
	}
}

func TestMultiSignTSAThenApproval(t *testing.T) {
	tsaServer := newMockTSAServer(t)
	defer tsaServer.Close()
	origTSA := TimestampHTTPClient
	TimestampHTTPClient = tsaServer.Client()
	t.Cleanup(func() { TimestampHTTPClient = origTSA })

	tsaURL := tsaServer.URL
	firstSD := approvalSignData(t, "With TSA", &TSA{URL: tsaURL})
	firstSD.DigestAlgorithm = crypto.SHA256 // mock TSA uses SHA256
	first := signToTemp(t, "../testfiles/testfile20.pdf", "multisign_tsa1_", firstSD)
	assertMultiSignPDF(t, first, 1)

	second := signToTemp(t, first.Name(), "multisign_tsa2_", approvalSignData(t, "Second without TSA", nil))
	assertMultiSignPDF(t, second, 2)
}

func TestMultiSignLTVThenApproval(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	input, err := os.Open("../testfiles/testfile20.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	finfo, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := pdf.NewReader(input, finfo.Size())
	if err != nil {
		t.Fatal(err)
	}

	ltvOut, err := os.CreateTemp("", "multisign_ltv_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(ltvOut.Name()) })
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	_, err = SignLTV(input, ltvOut, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:   "LTV first",
				Reason: "LTV",
				Date:   time.Now().UTC(),
			},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm:    crypto.SHA256,
		Signer:               key,
		Certificate:          cert,
		CertificateChains:    [][]*x509.Certificate{{cert}},
		RevocationFunction:   mockRevocationFunction,
	})
	if err != nil {
		t.Fatalf("SignLTV: %v", err)
	}
	if _, err := ltvOut.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	verifyAllSignaturesValid(t, ltvOut, 1)

	second := signToTemp(t, ltvOut.Name(), "multisign_ltv2_", approvalSignData(t, "After LTV", nil))
	assertMultiSignPDF(t, second, 2)
}

func TestMultiSignEncryptedPDF(t *testing.T) {
	encryptedPath := "../testfiles/testfile_encrypted.pdf"
	if _, err := os.Stat(encryptedPath); err != nil {
		t.Skip("testfile_encrypted.pdf missing; run encryption tests to generate it")
	}

	first := signToTemp(t, encryptedPath, "multisign_enc1_", approvalSignData(t, "Encrypted first", nil))
	assertMultiSignPDF(t, first, 1)

	second := signToTemp(t, first.Name(), "multisign_enc2_", approvalSignData(t, "Encrypted second", nil))
	assertMultiSignPDF(t, second, 2)
}
