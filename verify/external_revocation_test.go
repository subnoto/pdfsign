package verify

import (
	"crypto/x509"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPerformExternalOCSPCheck(t *testing.T) {
	// Create a test certificate with OCSP server
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(12345),
		OCSPServer:   []string{}, // Will be set by individual tests
	}

	issuer := &x509.Certificate{
		SerialNumber: big.NewInt(1),
	}

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		setupOptions    func(serverURL string) *VerifyOptions
		setupCert       func(serverURL string) *x509.Certificate
		expectChecked   bool
		expectValid     bool
		warningContains string
	}{
		{
			name: "External revocation disabled",
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: false,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				return cert
			},
			expectChecked:   true,
			expectValid:     false,
			warningContains: "external revocation checking is disabled",
		},
		{
			name: "No OCSP server URLs",
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.OCSPServer = []string{}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			warningContains: "certificate has no OCSP server URLs",
		},
		{
			name: "OCSP server returns invalid response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" {
						w.WriteHeader(http.StatusMethodNotAllowed)
						return
					}

					// Return a mock response that will fail parsing
					// In a real implementation, you'd need proper OCSP response signing
					w.Header().Set("Content-Type", "application/ocsp-response")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("mock-ocsp-response"))
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
					HTTPTimeout:                   5 * time.Second,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.OCSPServer = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			warningContains: "failed to parse OCSP response",
		},
		{
			name: "OCSP server returns error status",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.OCSPServer = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			warningContains: "returned status 500",
		},
		{
			name: "Custom HTTP client",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("custom-client-response"))
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
					HTTPClient: &http.Client{
						Timeout: 1 * time.Second,
					},
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.OCSPServer = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			warningContains: "failed to parse OCSP response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			var serverURL string

			if tt.setupServer != nil {
				server = tt.setupServer()
				defer server.Close()
				serverURL = server.URL
			}

			options := tt.setupOptions(serverURL)
			testCert := tt.setupCert(serverURL)

			// Use a mock OCSP request function for all cases except those that expect error due to disabled/external
			var ocspRequestFunc OCSPRequestFunc
			if tt.name != "External revocation disabled" && tt.name != "No OCSP server URLs" {
				ocspRequestFunc = func(cert, issuer *x509.Certificate) ([]byte, error) {
					return []byte("dummy-ocsp-request"), nil
				}
			}

			result := performExternalOCSPCheckWithFunc(testCert, issuer, options, ocspRequestFunc)

			if result.Checked != tt.expectChecked {
				t.Errorf("Expected Checked=%v, got %v", tt.expectChecked, result.Checked)
			}
			if result.Valid != tt.expectValid {
				t.Errorf("Expected Valid=%v, got %v", tt.expectValid, result.Valid)
			}
			if tt.warningContains != "" {
				if result.Warning == "" {
					t.Errorf("Expected warning containing '%s', but got empty warning", tt.warningContains)
				} else if !containsString(result.Warning, tt.warningContains) {
					t.Errorf("Expected warning to contain '%s', got: %s", tt.warningContains, result.Warning)
				}
			}
		})
	}
}

func TestPerformExternalCRLCheck(t *testing.T) {
	// Create a test certificate with CRL distribution points
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(12345),
	}

	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		setupOptions    func(serverURL string) *VerifyOptions
		setupCert       func(serverURL string) *x509.Certificate
		expectChecked   bool
		expectValid     bool
		expectRevoked   bool
		warningContains string
	}{
		{
			name: "External revocation disabled",
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: false,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				return cert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "external revocation checking is disabled",
		},
		{
			name: "No CRL distribution points",
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.CRLDistributionPoints = []string{}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "certificate has no CRL distribution points",
		},
		{
			name: "CRL server returns error status",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.CRLDistributionPoints = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "returned status 404",
		},
		{
			name: "CRL server returns invalid CRL",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("invalid-crl-data"))
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.CRLDistributionPoints = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "failed to parse CRL",
		},
		{
			name: "Multiple CRL URLs with first failing",
			setupServer: func() *httptest.Server {
				// Create two servers - first fails, second works
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("invalid-crl-data"))
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.CRLDistributionPoints = []string{
					"http://invalid-url.example.com/crl",
					serverURL,
				}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "failed to parse CRL",
		},
		{
			name: "Custom HTTP client with timeout",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("invalid-crl-data"))
				}))
			},
			setupOptions: func(serverURL string) *VerifyOptions {
				return &VerifyOptions{
					EnableExternalRevocationCheck: true,
					HTTPClient: &http.Client{
						Timeout: 1 * time.Second,
					},
				}
			},
			setupCert: func(serverURL string) *x509.Certificate {
				testCert := *cert
				testCert.CRLDistributionPoints = []string{serverURL}
				return &testCert
			},
			expectChecked:   true,
			expectValid:     false,
			expectRevoked:   false,
			warningContains: "failed to parse CRL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			var serverURL string

			if tt.setupServer != nil {
				server = tt.setupServer()
				defer server.Close()
				serverURL = server.URL
			}

			options := tt.setupOptions(serverURL)
			testCert := tt.setupCert(serverURL)

			result := performExternalCRLCheck(testCert, options)

			if result.Checked != tt.expectChecked {
				t.Errorf("Expected Checked=%v, got %v", tt.expectChecked, result.Checked)
			}
			if result.Valid != tt.expectValid {
				t.Errorf("Expected Valid=%v, got %v", tt.expectValid, result.Valid)
			}
			if result.IsRevoked != tt.expectRevoked {
				t.Errorf("Expected IsRevoked=%v, got %v", tt.expectRevoked, result.IsRevoked)
			}
			if tt.warningContains != "" {
				if result.Warning == "" {
					t.Errorf("Expected warning containing '%s', but got empty warning", tt.warningContains)
				} else if !containsString(result.Warning, tt.warningContains) {
					t.Errorf("Expected warning to contain '%s', got: %s", tt.warningContains, result.Warning)
				}
			}
			if tt.expectRevoked && result.RevocationTime == nil {
				t.Error("Expected revocation time when certificate is revoked")
			}
		})
	}
}

// TestExternalRevocationWithTestFile51 tests external revocation checking with testfile51.pdf
func TestExternalRevocationWithTestFile51(t *testing.T) {
	testFilePath := filepath.Join("..", "testfiles", "testfile51.pdf")

	// Check if test file exists
	if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
		t.Skipf("Test file %s does not exist", testFilePath)
	}

	// Open the test file
	file, err := os.Open(testFilePath)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Logf("Warning: failed to close file: %v", err)
		}
	}()

	// Verify the file to extract certificates
	options := DefaultVerifyOptions()
	options.EnableExternalRevocationCheck = false // Don't make real network calls yet
	response, err := VerifyFileWithOptions(file, options)
	if err != nil {
		t.Fatalf("Failed to verify file: %v", err)
	}

	if len(response.Signatures) == 0 {
		t.Fatal("No signatures found in testfile51.pdf")
	}

	// Find certificates with revocation URLs
	var certsWithOCSP []*x509.Certificate
	var certsWithCRL []*x509.Certificate
	issuerCerts := make(map[string]*x509.Certificate)

	for _, sig := range response.Signatures {
		for _, certInfo := range sig.Validation.Certificates {
			cert := certInfo.Certificate
			if cert == nil {
				continue
			}

			// Store issuer certs by subject key identifier or serial
			issuerKey := cert.Issuer.String()
			issuerCerts[issuerKey] = cert

			// Check for OCSP URLs
			if len(cert.OCSPServer) > 0 {
				certsWithOCSP = append(certsWithOCSP, cert)
				t.Logf("Found certificate with OCSP URL: %s (OCSP: %v)", cert.Subject.CommonName, cert.OCSPServer)
			}

			// Check for CRL URLs
			if len(cert.CRLDistributionPoints) > 0 {
				certsWithCRL = append(certsWithCRL, cert)
				t.Logf("Found certificate with CRL URL: %s (CRL: %v)", cert.Subject.CommonName, cert.CRLDistributionPoints)
			}
		}
	}

	if len(certsWithOCSP) == 0 && len(certsWithCRL) == 0 {
		t.Skip("testfile51.pdf does not contain certificates with revocation URLs")
	}

	// Test OCSP external revocation if we have OCSP URLs
	if len(certsWithOCSP) > 0 {
		t.Run("OCSP external revocation", func(t *testing.T) {
			testCert := certsWithOCSP[0]

			// Find issuer certificate
			var issuer *x509.Certificate
			for _, cert := range issuerCerts {
				if cert.Subject.String() == testCert.Issuer.String() {
					issuer = cert
					break
				}
			}
			if issuer == nil {
				// Try to find issuer from certificate chain
				for _, sig := range response.Signatures {
					for _, certInfo := range sig.Validation.Certificates {
						if certInfo.Certificate != nil && certInfo.Certificate.Subject.String() == testCert.Issuer.String() {
							issuer = certInfo.Certificate
							break
						}
					}
					if issuer != nil {
						break
					}
				}
			}

			// Create a mock OCSP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				// Return a mock response that will fail parsing (expected)
				w.Header().Set("Content-Type", "application/ocsp-response")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("mock-ocsp-response"))
			}))
			defer server.Close()

			// Test with external revocation enabled
			options := &VerifyOptions{
				EnableExternalRevocationCheck: true,
				HTTPTimeout:                   5 * time.Second,
			}

			// Mock the OCSP request function to use our test server
			ocspRequestFunc := func(cert, issuer *x509.Certificate) ([]byte, error) {
				return []byte("dummy-ocsp-request"), nil
			}

			// Temporarily replace the OCSP server URL with our test server
			originalOCSP := testCert.OCSPServer
			testCert.OCSPServer = []string{server.URL}
			defer func() {
				testCert.OCSPServer = originalOCSP
			}()

			result := performExternalOCSPCheckWithFunc(testCert, issuer, options, ocspRequestFunc)

			// We expect the check to be attempted but fail because the mock response won't parse correctly
			if !result.Checked {
				t.Error("Expected Checked=true, but got false")
			}
			if result.Valid {
				t.Error("Expected Valid=false because mock response won't parse, but got true")
			}
			if result.Warning == "" {
				t.Error("Expected warning message, but got empty")
			} else if !containsString(result.Warning, "failed to parse OCSP response") {
				t.Errorf("Expected warning to contain 'failed to parse OCSP response', got: %s", result.Warning)
			}
		})
	}

	// Test CRL external revocation if we have CRL URLs
	if len(certsWithCRL) > 0 {
		t.Run("CRL external revocation", func(t *testing.T) {
			testCert := certsWithCRL[0]

			// Create a mock CRL server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return invalid CRL data (expected to fail parsing)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("invalid-crl-data"))
			}))
			defer server.Close()

			// Test with external revocation enabled
			options := &VerifyOptions{
				EnableExternalRevocationCheck: true,
				HTTPTimeout:                   5 * time.Second,
			}

			// Temporarily replace the CRL URL with our test server
			originalCRL := testCert.CRLDistributionPoints
			testCert.CRLDistributionPoints = []string{server.URL}
			defer func() {
				testCert.CRLDistributionPoints = originalCRL
			}()

			result := performExternalCRLCheck(testCert, options)

			// We expect the check to be attempted but fail because the mock CRL won't parse correctly
			if !result.Checked {
				t.Error("Expected Checked=true, but got false")
			}
			if result.Valid {
				t.Error("Expected Valid=false because mock CRL won't parse, but got true")
			}
			if result.Warning == "" {
				t.Error("Expected warning message, but got empty")
			} else if !containsString(result.Warning, "failed to parse CRL") {
				t.Errorf("Expected warning to contain 'failed to parse CRL', got: %s", result.Warning)
			}
		})
	}
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsStringHelper(s, substr))))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
