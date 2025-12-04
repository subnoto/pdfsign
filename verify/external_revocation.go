package verify

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/crypto/ocsp"
)

// OCSPRequestFunc allows mocking OCSP request creation for tests
type OCSPRequestFunc func(cert, issuer *x509.Certificate) ([]byte, error)

// performExternalOCSPCheck performs an external OCSP check for the given certificate
func performExternalOCSPCheck(cert, issuer *x509.Certificate, options *VerifyOptions) ExternalOCSPResult {
	return performExternalOCSPCheckWithFunc(cert, issuer, options, nil)
}

// performExternalOCSPCheckWithFunc allows injecting a custom OCSP request function for testing
func performExternalOCSPCheckWithFunc(cert, issuer *x509.Certificate, options *VerifyOptions, ocspRequestFunc OCSPRequestFunc) ExternalOCSPResult {
	result := ExternalOCSPResult{
		Checked: false,
		Valid:   false,
	}

	if !options.EnableExternalRevocationCheck {
		result.Checked = true
		result.Warning = "external revocation checking is disabled"
		return result
	}

	if len(cert.OCSPServer) == 0 {
		result.Checked = true
		result.Warning = "certificate has no OCSP server URLs"
		return result
	}

	result.Checked = true

	// Create OCSP request (use injected func if provided)
	var ocspReq []byte
	var err error
	if ocspRequestFunc != nil {
		ocspReq, err = ocspRequestFunc(cert, issuer)
	} else {
		ocspReq, err = ocsp.CreateRequest(cert, issuer, nil)
	}
	if err != nil {
		result.Warning = fmt.Sprintf("failed to create OCSP request: %v", err)
		return result
	}

	// Get HTTP client with timeout and proxy support
	client := getHTTPClient(options)

	// Try each OCSP server URL
	var lastErr error
	for _, serverURL := range cert.OCSPServer {
		resp, err := client.Post(serverURL, "application/ocsp-request", bytes.NewReader(ocspReq))
		if err != nil {
			lastErr = fmt.Errorf("failed to contact OCSP server %s: %v", serverURL, err)
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				// Log error but don't fail the operation
				lastErr = fmt.Errorf("failed to close response body: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("OCSP server %s returned status %d", serverURL, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read OCSP response from %s: %v", serverURL, err)
			continue
		}

		ocspResp, err := ocsp.ParseResponse(body, issuer)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse OCSP response from %s: %v", serverURL, err)
			continue
		}

		// Successfully got OCSP response
		result.Valid = true
		result.Response = ocspResp
		return result
	}

	// All attempts failed
	if lastErr != nil {
		result.Warning = lastErr.Error()
	} else {
		result.Warning = "failed to retrieve OCSP response from all servers"
	}
	return result
}

// performExternalCRLCheck performs an external CRL check for the given certificate
func performExternalCRLCheck(cert *x509.Certificate, options *VerifyOptions) ExternalCRLResult {
	result := ExternalCRLResult{
		Checked:   false,
		Valid:     false,
		IsRevoked: false,
	}

	if !options.EnableExternalRevocationCheck {
		result.Checked = true
		result.Warning = "external revocation checking is disabled"
		return result
	}

	if len(cert.CRLDistributionPoints) == 0 {
		result.Checked = true
		result.Warning = "certificate has no CRL distribution points"
		return result
	}

	result.Checked = true

	// Get HTTP client with timeout and proxy support
	client := getHTTPClient(options)

	// Try each CRL distribution point
	var lastErr error
	for _, crlURL := range cert.CRLDistributionPoints {
		resp, err := client.Get(crlURL)
		if err != nil {
			lastErr = fmt.Errorf("failed to download CRL from %s: %v", crlURL, err)
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				// Log error but don't fail the operation
				lastErr = fmt.Errorf("failed to close response body: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("CRL server %s returned status %d", crlURL, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read CRL from %s: %v", crlURL, err)
			continue
		}

		crl, err := x509.ParseRevocationList(body)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse CRL from %s: %v", crlURL, err)
			continue
		}

		// Successfully parsed CRL
		result.Valid = true

		// Check if certificate is revoked
		for _, revokedCert := range crl.RevokedCertificateEntries {
			if revokedCert.SerialNumber.Cmp(cert.SerialNumber) == 0 {
				result.IsRevoked = true
				result.RevocationTime = &revokedCert.RevocationTime
				return result // Certificate is revoked
			}
		}

		// Successfully checked CRL, certificate not revoked
		return result
	}

	// All attempts failed
	if lastErr != nil {
		result.Warning = lastErr.Error()
	} else {
		result.Warning = "failed to retrieve CRL from all distribution points"
	}
	return result
}
