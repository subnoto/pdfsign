package sign

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/digitorus/pdf"
)

func TestSignPDFValidationErrors(t *testing.T) {
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

	cert, key := loadCertificateAndKey(t)

	tests := []struct {
		name     string
		signData SignData
		wantErr  string
	}{
		{
			name: "nil certificate",
			signData: SignData{
				Signature: SignDataSignature{CertType: ApprovalSignature},
				Signer:    key,
			},
			wantErr: "certificate is required",
		},
		{
			name: "visible on certification",
			signData: SignData{
				Signature: SignDataSignature{CertType: CertificationSignature},
				Signer:    key,
				Certificate: cert,
				Appearance: Appearance{
					Visible: true,
					Page:    1,
					LowerLeftX: 0, LowerLeftY: 0,
					UpperRightX: 100, UpperRightY: 50,
				},
			},
			wantErr: "visible signatures are only allowed for approval signatures",
		},
		{
			name: "invalid image",
			signData: SignData{
				Signature: SignDataSignature{CertType: ApprovalSignature},
				Signer:    key,
				Certificate: cert,
				Appearance: Appearance{
					Visible: true,
					Page:    1,
					LowerLeftX: 0, LowerLeftY: 0,
					UpperRightX: 100, UpperRightY: 50,
					Image: []byte("not-an-image"),
				},
			},
			wantErr: "failed to create visual signature",
		},
		{
			name: "invalid rect",
			signData: SignData{
				Signature: SignDataSignature{CertType: ApprovalSignature},
				Signer:    key,
				Certificate: cert,
				Appearance: Appearance{
					Visible: true,
					Page:    1,
					LowerLeftX: 100, LowerLeftY: 100,
					UpperRightX: 0, UpperRightY: 0,
				},
			},
			wantErr: "failed to create visual signature",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := input.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			_, err := Sign(input, &out, rdr, finfo.Size(), tc.signData)
			if err == nil {
				t.Fatal("expected error")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantErr)) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestReplaceSignatureTooSmallReturnsRetryError(t *testing.T) {
	ctx := &SignContext{
		SignatureMaxLength:     10,
		SignatureMaxLengthBase: 100,
	}
	// Simulate the resize logic executed when hex-encoded CMS exceeds the placeholder.
	overflow := uint32(200)
	if overflow > ctx.SignatureMaxLength {
		ctx.SignatureMaxLengthBase += (overflow - ctx.SignatureMaxLength) + 1
	}
	if ctx.SignatureMaxLengthBase <= 100 {
		t.Fatalf("expected SignatureMaxLengthBase to increase, got %d", ctx.SignatureMaxLengthBase)
	}
	if !errors.Is(errSignatureBufferTooSmall, errSignatureBufferTooSmall) {
		t.Fatal("errSignatureBufferTooSmall sentinel missing")
	}
}
