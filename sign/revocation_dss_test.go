package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
	"github.com/digitorus/timestamp"
	"github.com/subnoto/pdfsign/revocation"
)

func signTestPDF(t *testing.T) []byte {
	t.Helper()
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
	var buf bytes.Buffer
	_, err = Sign(input, &buf, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "DSS Test",
				Date: time.Now(),
			},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm:     crypto.SHA256,
		Signer:              key,
		Certificate:         cert,
		CertificateChains:   [][]*x509.Certificate{{cert}},
		RevocationFunction:  mockRevocationFunction,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return buf.Bytes()
}

func mockRevocationFunction(cert, issuer *x509.Certificate, ia *revocation.InfoArchival) error {
	return ia.AddOCSP([]byte{0x30, 0x03, 0x02, 0x01, 0x00})
}

func TestBestEffortEmbedRevocationStatusFunction(t *testing.T) {
	cert := &x509.Certificate{OCSPServer: []string{"http://127.0.0.1:1/"}}
	var ia revocation.InfoArchival
	if err := BestEffortEmbedRevocationStatusFunction(cert, nil, &ia); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAddValidationData(t *testing.T) {
	cert, _ := loadCertificateAndKey(t)
	signed := signTestPDF(t)

	t.Run("xref_table", func(t *testing.T) {
		out, err := AddValidationData(signed, []*x509.Certificate{cert}, [][]byte{{0x30, 0x03, 0x02, 0x01, 0x00}}, nil, nil)
		if err != nil {
			t.Fatalf("AddValidationData: %v", err)
		}
		s := string(out)
		for _, want := range []string{"/Type /DSS", "/Certs", "/DSS", "startxref", "/VRI"} {
			if !strings.Contains(s, want) {
				t.Fatalf("missing %q in output", want)
			}
		}
		if !strings.Contains(s, "xref\n") {
			t.Fatal("expected classic xref table incremental update")
		}
	})

	t.Run("signed_xref_stream_source", func(t *testing.T) {
		// testfile12 uses a cross-reference stream; after signing the incremental
		// update may be a classic xref table, but AddValidationData must still succeed.
		input, err := os.Open("../testfiles/testfile12.pdf")
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
		cert, key := loadCertificateAndKey(t)
		var signedBuf bytes.Buffer
		_, err = Sign(input, &signedBuf, rdr, finfo.Size(), SignData{
			Signature: SignDataSignature{
				Info:     SignDataSignatureInfo{Name: "Stream xref"},
				CertType: ApprovalSignature,
			},
			DigestAlgorithm:    crypto.SHA256,
			Signer:             key,
			Certificate:        cert,
			CertificateChains:  [][]*x509.Certificate{{cert}},
			RevocationFunction: mockRevocationFunction,
		})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		out, err := AddValidationData(signedBuf.Bytes(), []*x509.Certificate{cert}, [][]byte{{0x30, 0x03, 0x02, 0x01, 0x00}}, nil, nil)
		if err != nil {
			t.Fatalf("AddValidationData: %v", err)
		}
		if !strings.Contains(string(out), "/Type /DSS") {
			t.Fatal("expected DSS dictionary in output")
		}
	})
}

func TestSignLTV(t *testing.T) {
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

	var out bytes.Buffer
	_, err = SignLTV(input, &out, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info:     SignDataSignatureInfo{Name: "LTV Test"},
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
	if !strings.Contains(out.String(), "/Type /DSS") {
		t.Fatalf("SignLTV output missing DSS: len=%d", out.Len())
	}
	if !strings.Contains(out.String(), "/VRI") {
		t.Fatal("SignLTV output missing /VRI dictionary")
	}
}

func TestSignLTVEncryptedPDF(t *testing.T) {
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

	var out bytes.Buffer
	_, err = SignLTV(input, &out, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info:     SignDataSignatureInfo{Name: "Encrypted LTV"},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm:    crypto.SHA256,
		Signer:             key,
		Certificate:        cert,
		CertificateChains:  [][]*x509.Certificate{{cert}},
		RevocationFunction: mockRevocationFunction,
	})
	if err != nil {
		t.Fatalf("SignLTV encrypted: %v", err)
	}
	s := out.String()
	for _, want := range []string{"/Type /DSS", "/VRI", "/Encrypt"} {
		if !strings.Contains(s, want) {
			t.Fatalf("encrypted LTV output missing %q", want)
		}
	}
}

func TestEmbedOCSPUsesPOST(t *testing.T) {
	var method string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x30, 0x03, 0x02, 0x01, 0x06})
	}))
	defer server.Close()

	orig := RevocationHTTPClient
	RevocationHTTPClient = server.Client()
	defer func() { RevocationHTTPClient = orig }()

	cert, _ := loadCertificateAndKey(t)
	cert.OCSPServer = []string{server.URL}
	var ia revocation.InfoArchival
	err := embedOCSPRevocationStatus(cert, cert, &ia)
	if err == nil {
		t.Fatal("expected OCSP parse error for dummy response")
	}
	if method != "POST" {
		t.Fatalf("expected POST to OCSP responder, got %q", method)
	}
}

func newMockTSAServer(t *testing.T) *httptest.Server {
	t.Helper()
	tsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Mock TSA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &tsaKey.PublicKey, tsaKey)
	if err != nil {
		t.Fatal(err)
	}
	tsaCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req, err := timestamp.ParseRequest(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ts := timestamp.Timestamp{
			HashAlgorithm: crypto.SHA256,
			HashedMessage: req.HashedMessage,
			Time:          time.Now().UTC(),
			Policy:        asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 1},
		}
		resp, err := ts.CreateResponseWithOpts(tsaCert, tsaKey, crypto.SHA256)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
}

func TestSignLTA(t *testing.T) {
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

	tsaServer := newMockTSAServer(t)
	defer tsaServer.Close()

	origTSA := TimestampHTTPClient
	TimestampHTTPClient = tsaServer.Client()
	defer func() { TimestampHTTPClient = origTSA }()

	var out bytes.Buffer
	_, err = SignLTA(input, &out, rdr, finfo.Size(), SignData{
		Signature: SignDataSignature{
			Info:     SignDataSignatureInfo{Name: "LTA Test"},
			CertType: ApprovalSignature,
		},
		DigestAlgorithm:    crypto.SHA256,
		Signer:               key,
		Certificate:          cert,
		CertificateChains:    [][]*x509.Certificate{{cert}},
		RevocationFunction:   mockRevocationFunction,
		TSA:                  TSA{URL: tsaServer.URL},
	})
	if err != nil {
		t.Fatalf("SignLTA: %v", err)
	}

	s := out.String()
	for _, want := range []string{"/Type /DSS", "/Type /DocTimeStamp", "/SubFilter /ETSI.RFC3161"} {
		if !strings.Contains(s, want) {
			t.Fatalf("LTA output missing %q", want)
		}
	}
}
