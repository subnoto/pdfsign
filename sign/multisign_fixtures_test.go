package sign

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

func writeMultiSignFixture(t *testing.T, inputPath, outputName string, steps []SignData) {
	t.Helper()
	current := inputPath
	var temps []string
	t.Cleanup(func() {
		for _, p := range temps {
			_ = os.Remove(p)
		}
	})

	for i, sd := range steps {
		var outPath string
		if i == len(steps)-1 {
			out, cleanup := openSuccessOutput(t, outputName)
			outPath = out.Name()
			if err := out.Close(); err != nil {
				cleanup()
				t.Fatal(err)
			}
			if !testing.Verbose() {
				temps = append(temps, outPath)
			}
		} else {
			tmp, err := os.CreateTemp("", fmt.Sprintf("multisign_%d_", i))
			if err != nil {
				t.Fatal(err)
			}
			if err := tmp.Close(); err != nil {
				t.Fatal(err)
			}
			outPath = tmp.Name()
			temps = append(temps, outPath)
		}

		if _, err := SignFile(current, outPath, sd); err != nil {
			t.Fatalf("SignFile step %d: %v", i+1, err)
		}
		current = outPath
	}

	reopen, err := os.Open(current)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopen.Close() }()
	verifyAllSignaturesValid(t, reopen, len(steps))
	assertPAdESSignatureObjects(t, openPDFReader(t, current))
}

func TestMultiSignFixtures(t *testing.T) {
	image, err := os.ReadFile("../testfiles/pdfsign-signature.jpg")
	if err != nil {
		t.Fatal(err)
	}

	tsaServer := newMockTSAServer(t)
	defer tsaServer.Close()
	origTSA := TimestampHTTPClient
	TimestampHTTPClient = tsaServer.Client()
	t.Cleanup(func() { TimestampHTTPClient = origTSA })

	cert, key := loadCertificateAndKey(t)

	t.Run("three_approvals", func(t *testing.T) {
		steps := make([]SignData, 3)
		for i := range steps {
			steps[i] = approvalSignData(t, fmt.Sprintf("Approval %d", i+1), nil)
		}
		writeMultiSignFixture(t, "../testfiles/testfile20.pdf", "testfile20_TestMultiSignThreeApprovals.pdf", steps)
	})

	t.Run("visible_twice", func(t *testing.T) {
		steps := make([]SignData, 2)
		for i := range steps {
			sd := approvalSignData(t, fmt.Sprintf("Visible %d", i+1), nil)
			sd.Appearance = Appearance{
				Visible:     true,
				Page:        1,
				LowerLeftX:  50 + float64(i*250),
				LowerLeftY:  50,
				UpperRightX: 250 + float64(i*250),
				UpperRightY: 125,
				Image:       image,
			}
			steps[i] = sd
		}
		writeMultiSignFixture(t, "../testfiles/testfile12.pdf", "testfile12_TestMultiSignVisibleTwice.pdf", steps)
	})

	t.Run("tsa_then_approval", func(t *testing.T) {
		first := approvalSignData(t, "With TSA", &TSA{URL: tsaServer.URL})
		first.DigestAlgorithm = crypto.SHA256
		writeMultiSignFixture(t, "../testfiles/testfile20.pdf", "testfile20_TestMultiSignTSAThenApproval.pdf", []SignData{
			first,
			approvalSignData(t, "Second without TSA", nil),
		})
	})

	t.Run("ltv_then_approval", func(t *testing.T) {
		inputPath := "../testfiles/testfile20.pdf"
		input, err := os.Open(inputPath)
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
			Signer:             key,
			Certificate:        cert,
			CertificateChains:  [][]*x509.Certificate{{cert}},
			RevocationFunction: mockRevocationFunction,
		})
		if err != nil {
			t.Fatalf("SignLTV: %v", err)
		}
		if err := ltvOut.Close(); err != nil {
			t.Fatal(err)
		}

		writeMultiSignFixture(t, ltvOut.Name(), "testfile20_TestMultiSignLTVThenApproval.pdf", []SignData{
			approvalSignData(t, "After LTV", nil),
		})

		if testing.Verbose() {
			assertLTVPDFMarkers(t, filepath.Join("../testfiles/success", "testfile20_TestMultiSignLTVThenApproval.pdf"))
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		encryptedPath := "../testfiles/testfile_encrypted.pdf"
		if _, err := os.Stat(encryptedPath); err != nil {
			t.Skip("testfile_encrypted.pdf missing")
		}
		writeMultiSignFixture(t, encryptedPath, "testfile_encrypted_TestMultiSignTwice.pdf", []SignData{
			approvalSignData(t, "Encrypted first", nil),
			approvalSignData(t, "Encrypted second", nil),
		})
	})
}
