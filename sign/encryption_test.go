package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func generate4096Cert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Large RSA Test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert, key
}

func TestSignatureMaxLengthLargeRSA(t *testing.T) {
	cert, key := generate4096Cert(t)
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

	var out bytes.Buffer
	_, err = Sign(input, &out, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "Large RSA",
				Date: time.Now(),
			},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm: crypto.SHA256,
		Signer:          key,
		Certificate:     cert,
		CertificateChains: [][]*x509.Certificate{
			{cert},
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "negative Repeat count") {
			t.Fatalf("signature length too small for 4096-bit RSA: %v", err)
		}
		t.Fatalf("Sign: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("empty signed output")
	}
}

func TestEncryptPdfString(t *testing.T) {
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	ctx := &SignContext{
		encryption: &EncryptionContext{
			Key:        key,
			UseAES:     true,
			EncVersion: 4,
		},
	}

	plain, err := ctx.encryptPdfString(99, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "<") || !strings.HasSuffix(plain, ">") {
		t.Fatalf("encrypted string should be hex-wrapped, got %q", plain)
	}

	noEnc, err := (&SignContext{}).encryptPdfString(99, "test")
	if err != nil {
		t.Fatal(err)
	}
	if noEnc != pdfString("test") {
		t.Fatalf("without encryption expected pdfString, got %q", noEnc)
	}
}

func TestSignEncryptedPDF(t *testing.T) {
	cert, key := loadCertificateAndKey(t)

	inputPath := "../testfiles/testfile_encrypted.pdf"
	input, err := os.Open(inputPath)
	if err != nil {
		t.Skipf("encrypted fixture missing: %v", err)
	}
	defer func() { _ = input.Close() }()

	finfo, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}

	rdr, err := pdf.NewReaderEncrypted(input, finfo.Size(), func() string { return "" })
	if err != nil {
		t.Fatalf("NewReaderEncrypted: %v", err)
	}
	if rdr.EncryptionKey() == nil {
		t.Fatal("expected encryption key")
	}

	var out bytes.Buffer
	_, err = Sign(input, &out, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name:   "Encrypted Signer",
				Reason: "encryption test",
				Date:   time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
			},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm: crypto.SHA256,
		Signer:          key,
		Certificate:     cert,
		CertificateChains: [][]*x509.Certificate{
			{cert},
		},
	})
	if err != nil {
		t.Fatalf("Sign encrypted PDF: %v", err)
	}

	signed := out.Bytes()
	if !bytes.Contains(signed, []byte("/Encrypt")) {
		t.Error("signed encrypted PDF should preserve /Encrypt in incremental trailer")
	}
	if strings.Contains(string(signed), "/Name (Encrypted Signer)") {
		t.Error("signature /Name must be encrypted in output, not plaintext")
	}
	if !strings.Contains(string(signed), "/Name <") {
		t.Error("expected hex-encoded encrypted /Name in signed output")
	}
	if strings.Contains(string(signed), "/Reason (encryption test)") {
		t.Error("signature /Reason must be encrypted in output, not plaintext")
	}

	outRdr, err := pdf.NewReaderEncrypted(bytes.NewReader(signed), int64(len(signed)), func() string { return "" })
	if err != nil {
		t.Fatalf("re-open signed encrypted PDF: %v", err)
	}

	contents, ok := signatureContentsFromReader(outRdr)
	if !ok {
		t.Fatal("signed encrypted PDF should contain a signature dictionary")
	}
	if len(contents) < 2 || contents[0] != 0x30 {
		t.Fatalf("signature Contents should be raw PKCS#7 DER, got prefix %x", contents[:min(8, len(contents))])
	}
}

func TestEncryptedSignaturePlaceholder(t *testing.T) {
	inputPath := "../testfiles/testfile_encrypted.pdf"
	input, err := os.Open(inputPath)
	if err != nil {
		t.Skipf("encrypted fixture missing: %v", err)
	}
	defer func() { _ = input.Close() }()

	finfo, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}

	rdr, err := pdf.NewReaderEncrypted(input, finfo.Size(), func() string { return "" })
	if err != nil {
		t.Fatalf("NewReaderEncrypted: %v", err)
	}

	ctx := SignContext{
		PDFReader: rdr,
		InputFile: input,
		SignData: SignData{
			Signature: SignDataSignature{
				Info: SignDataSignatureInfo{
					Name:   "Encrypted Signer",
					Reason: "placeholder test",
				},
				CertType: ApprovalSignature,
			},
		},
	}
	ctx.SignData.objectId = uint32(rdr.XrefInformation.ItemCount) + 3
	if rdr.EncryptionKey() != nil {
		ctx.encryption = &EncryptionContext{
			Key:        rdr.EncryptionKey(),
			UseAES:     rdr.UseAES(),
			EncVersion: rdr.EncVersion(),
		}
	}

	placeholderBytes, err := ctx.createSignaturePlaceholder()
	if err != nil {
		t.Fatal(err)
	}
	placeholder := string(placeholderBytes)
	if strings.Contains(placeholder, "(Encrypted Signer)") {
		t.Fatalf("placeholder /Name must be encrypted:\n%s", placeholder)
	}
	if !strings.Contains(placeholder, "/Name <") {
		t.Fatalf("expected encrypted /Name in placeholder:\n%s", placeholder)
	}
}

func signatureContentsFromReader(r *pdf.Reader) ([]byte, bool) {
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
		raw := val.Key("Contents").RawString()
		if raw == "" {
			continue
		}
		return []byte(raw), true
	}
	return nil, false
}
