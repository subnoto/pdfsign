package sign

import (
	"crypto"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func baseLTVSignData(t *testing.T) SignData {
	t.Helper()
	tsaServer := newMockTSAServer(t)
	t.Cleanup(tsaServer.Close)
	origTSA := TimestampHTTPClient
	TimestampHTTPClient = tsaServer.Client()
	t.Cleanup(func() { TimestampHTTPClient = origTSA })
	cert, key := loadCertificateAndKey(t)
	return SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:     "LTV Fixture",
				Location: "Test",
				Reason:   "LTV fixture generation",
				Date:     time.Now().UTC(),
			},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm:    crypto.SHA256,
		Signer:             key,
		Certificate:        cert,
		CertificateChains:  [][]*x509.Certificate{{cert}},
		RevocationFunction: mockRevocationFunction,
		TSA:                TSA{URL: tsaServer.URL},
	}
}

// assertLTVPDFMarkers checks ISO 32000 DSS / VRI markers for PAdES B-LT.
func assertLTVPDFMarkers(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"/Type /DSS", "/VRI", "/Type /Sig", "/SubFilter /adbe.pkcs7.detached"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%s: missing %q in LTV output", filepath.Base(path), want)
		}
	}
}

// assertLTAPDFMarkers checks PAdES B-LTA archive timestamp in addition to LTV markers.
func assertLTAPDFMarkers(t *testing.T, path string) {
	t.Helper()
	assertLTVPDFMarkers(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"/Type /DocTimeStamp", "/SubFilter /ETSI.RFC3161"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%s: missing %q in LTA output", filepath.Base(path), want)
		}
	}
}

func writeLTVFixture(t *testing.T, inputPath, outputName string, encrypted bool) {
	t.Helper()
	signData := baseLTVSignData(t)

	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("open input: %v", err)
	}
	defer func() { _ = input.Close() }()

	finfo, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}

	var rdr *pdf.Reader
	if encrypted {
		rdr, err = pdf.NewReaderEncrypted(input, finfo.Size(), func() string { return "" })
	} else {
		rdr, err = pdf.NewReader(input, finfo.Size())
	}
	if err != nil {
		t.Fatalf("pdf reader: %v", err)
	}

	outputFile, cleanup := openSuccessOutput(t, outputName)
	defer cleanup()

	_, err = SignLTV(input, outputFile, rdr, finfo.Size(), signData)
	if err != nil {
		t.Fatalf("SignLTV: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		t.Fatal(err)
	}

	assertLTVPDFMarkers(t, outputFile.Name())
	reopen, err := os.Open(outputFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopen.Close() }()
	verifySignedFile(t, reopen, outputName)
}

func writeLTAFixture(t *testing.T, inputPath, outputName string) {
	t.Helper()
	signData := baseLTVSignData(t)

	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("open input: %v", err)
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

	outputFile, cleanup := openSuccessOutput(t, outputName)
	defer cleanup()

	_, err = SignLTA(input, outputFile, rdr, finfo.Size(), signData)
	if err != nil {
		t.Fatalf("SignLTA: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		t.Fatal(err)
	}

	assertLTAPDFMarkers(t, outputFile.Name())
	reopen, err := os.Open(outputFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopen.Close() }()
	verifySignedFile(t, reopen, outputName)
}

func openSuccessOutput(t *testing.T, outputName string) (*os.File, func()) {
	t.Helper()
	if testing.Verbose() {
		path := filepath.Join("../testfiles/success", outputName)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		return f, func() {}
	}
	f, err := os.CreateTemp("", outputName+"_")
	if err != nil {
		t.Fatal(err)
	}
	return f, func() { _ = os.Remove(f.Name()) }
}

func TestSignLTVFixtures(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		encrypted bool
	}{
		{name: "testfile20", input: "../testfiles/testfile20.pdf"},
		{name: "gen_pdf14_acroform", input: "../testfiles/gen_pdf14_acroform.pdf"},
		{name: "testfile_encrypted", input: "../testfiles/testfile_encrypted.pdf", encrypted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.input); err != nil {
				t.Skipf("input missing: %v", err)
			}
			outputName := strings.TrimSuffix(filepath.Base(tc.input), ".pdf") + "_TestSignLTV.pdf"
			writeLTVFixture(t, tc.input, outputName, tc.encrypted)
		})
	}
}

func TestSignLTAFixtures(t *testing.T) {
	input := "../testfiles/testfile20.pdf"
	if _, err := os.Stat(input); err != nil {
		t.Skipf("input missing: %v", err)
	}
	writeLTAFixture(t, input, "testfile20_TestSignLTA.pdf")
}
