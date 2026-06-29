package sign

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/subnoto/pdfsign/revocation"
	"golang.org/x/crypto/ocsp"
)

func TestEmbedOCSPRevocationStatusValidResponse(t *testing.T) {
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuerTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, &issuerTemplate, &issuerTemplate, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		t.Fatal(err)
	}

	leafTemplate := issuerTemplate
	leafTemplate.SerialNumber = big.NewInt(2)
	leafTemplate.Subject = pkix.Name{CommonName: "Leaf"}
	leafTemplate.IsCA = false
	leafTemplate.KeyUsage = x509.KeyUsageDigitalSignature
	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, issuer, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}

	respDER, err := ocsp.CreateResponse(issuer, issuer, ocsp.Response{
		Status:       ocsp.Good,
		SerialNumber: leaf.SerialNumber,
		ThisUpdate:   time.Now().Add(-time.Minute),
		NextUpdate:   time.Now().Add(time.Hour),
	}, issuerKey)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respDER)
	}))
	defer server.Close()

	orig := RevocationHTTPClient
	RevocationHTTPClient = server.Client()
	defer func() { RevocationHTTPClient = orig }()

	leaf.OCSPServer = []string{server.URL}
	var ia revocation.InfoArchival
	if err := embedOCSPRevocationStatus(leaf, issuer, &ia); err != nil {
		t.Fatalf("embedOCSPRevocationStatus: %v", err)
	}
	if len(ia.OCSP) != 1 {
		t.Fatalf("expected 1 OCSP response, got %d", len(ia.OCSP))
	}
}

func TestEmbedCRLRevocationStatusValidResponse(t *testing.T) {
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuerTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CRL CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, &issuerTemplate, &issuerTemplate, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		t.Fatal(err)
	}

	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: time.Now().Add(-time.Minute),
		NextUpdate: time.Now().Add(time.Hour),
		RevokedCertificateEntries: []x509.RevocationListEntry{},
	}, issuer, issuerKey)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(crlDER)
	}))
	defer server.Close()

	orig := RevocationHTTPClient
	RevocationHTTPClient = server.Client()
	defer func() { RevocationHTTPClient = orig }()

	leaf := &x509.Certificate{
		CRLDistributionPoints: []string{server.URL},
	}

	var ia revocation.InfoArchival
	if err := embedCRLRevocationStatus(leaf, issuer, &ia); err != nil {
		t.Fatalf("embedCRLRevocationStatus: %v", err)
	}
	if len(ia.CRL) != 1 {
		t.Fatalf("expected 1 CRL, got %d", len(ia.CRL))
	}
}

func TestEmbedOCSPRejectsNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	orig := RevocationHTTPClient
	RevocationHTTPClient = server.Client()
	defer func() { RevocationHTTPClient = orig }()

	cert, _ := loadCertificateAndKey(t)
	cert.OCSPServer = []string{server.URL}
	var ia revocation.InfoArchival
	if err := embedOCSPRevocationStatus(cert, cert, &ia); err == nil {
		t.Fatal("expected error for non-OK OCSP status")
	}
}

func TestDefaultEmbedRevocationStatusFunction(t *testing.T) {
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuerTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "Default CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, &issuerTemplate, &issuerTemplate, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		t.Fatal(err)
	}

	leafTemplate := issuerTemplate
	leafTemplate.SerialNumber = big.NewInt(11)
	leafTemplate.Subject = pkix.Name{CommonName: "Default Leaf"}
	leafTemplate.IsCA = false
	leafTemplate.KeyUsage = x509.KeyUsageDigitalSignature
	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, issuer, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}

	ocspDER, err := ocsp.CreateResponse(issuer, issuer, ocsp.Response{
		Status:       ocsp.Good,
		SerialNumber: leaf.SerialNumber,
		ThisUpdate:   time.Now().Add(-time.Minute),
		NextUpdate:   time.Now().Add(time.Hour),
	}, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: time.Now().Add(-time.Minute),
		NextUpdate: time.Now().Add(time.Hour),
		RevokedCertificateEntries: []x509.RevocationListEntry{},
	}, issuer, issuerKey)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ocsp", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ocspDER)
	})
	mux.HandleFunc("/crl", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(crlDER)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	orig := RevocationHTTPClient
	RevocationHTTPClient = server.Client()
	defer func() { RevocationHTTPClient = orig }()

	leaf.OCSPServer = []string{server.URL + "/ocsp"}
	leaf.CRLDistributionPoints = []string{server.URL + "/crl"}

	var ia revocation.InfoArchival
	if err := DefaultEmbedRevocationStatusFunction(leaf, issuer, &ia); err != nil {
		t.Fatalf("DefaultEmbedRevocationStatusFunction: %v", err)
	}
	if len(ia.OCSP) == 0 {
		t.Fatal("expected OCSP data")
	}
}
