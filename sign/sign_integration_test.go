package sign

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/digitorus/pdf"
)

func decodeFieldNameFromPDF(raw string) string {
	return decodeFieldName(raw)
}

func TestSignFileInvalidTimezone(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "sign_tz_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, err = SignFile("../testfiles/testfile60.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "TZ Test",
				Date: time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
			},
			CertType: ApprovalSignature,
		},
		Signer:      key,
		Certificate: cert,
		Appearance: Appearance{
			SignerUID: "toto@toto.com",
			Timezone:  "Not/A/Zone",
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if !strings.Contains(err.Error(), "timezone") && !strings.Contains(err.Error(), "Unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignPDFPlainSignerUIDInitials(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "sign_plain_uid_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	// Field names on testfile50 embed hex(new@toto.com), not hex(newt@toto.com).
	uid := "new@toto.com"
	_, err = SignFile("../testfiles/testfile50.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "Newt Totoo",
				Date: time.Now(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:      key,
		Certificate: cert,
		Appearance: Appearance{
			SignerUID: uid,
		},
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	sf, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sf.Close() }()
	sfi, _ := sf.Stat()
	rdr, err := pdf.NewReader(sf, sfi.Size())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	fields := rdr.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		decoded := decodeFieldNameFromPDF(field.Key("T").RawString())
		if !strings.Contains(decoded, "6e657740746f746f2e636f6d") {
			continue
		}
		if strings.Contains(field.Key("V").RawString(), "NT") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected initials NT for plain uid %q", uid)
	}
}

func TestSignPDFDateStylesIntegration(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	uid := "toto@toto.com"
	signDate := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		appearance Appearance
		wantSub   string
	}{
		{
			name: "long french",
			appearance: Appearance{
				SignerUID: uid,
				DateStyle: DateStyleLong,
				Locale:    "fr-FR",
				Timezone:  "UTC",
			},
			wantSub: "mars",
		},
		{
			name: "date only",
			appearance: Appearance{
				SignerUID: uid,
				DateStyle: DateStyleDateOnly,
				Locale:    "en-US",
				Timezone:  "UTC",
			},
			wantSub: "2024",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp, err := os.CreateTemp("", "sign_date_style_")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(tmp.Name()) }()

			_, err = SignFile("../testfiles/testfile60.pdf", tmp.Name(), SignData{
				Signature: SignDataSignature{
					Info: SignDataSignatureInfo{
						Name: "Date Style",
						Date: signDate,
					},
					CertType: ApprovalSignature,
				},
				Signer:      key,
				Certificate: cert,
				Appearance:  tc.appearance,
			})
			if err != nil {
				t.Fatalf("sign: %v", err)
			}

			val, ok, err := filledDateFieldValueForUID(tmp.Name(), uid)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatal("date field not filled")
			}
			if !strings.Contains(strings.ToLower(val), strings.ToLower(tc.wantSub)) {
				t.Fatalf("field value %q missing %q", val, tc.wantSub)
			}
		})
	}
}

func TestSignPDFFilledFieldReadOnlyAndAppearance(t *testing.T) {
	cert, key := loadCertificateAndKey(t)
	tmp, err := os.CreateTemp("", "sign_readonly_")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	uid := "6e657740746f746f2e636f6d" // hex for newt@toto.com on testfile50
	_, err = SignFile("../testfiles/testfile50.pdf", tmp.Name(), SignData{
		Signature: SignDataSignature{
			Info: SignDataSignatureInfo{
				Name: "Newt Totoo",
				Date: time.Now(),
			},
			CertType:   CertificationSignature,
			DocMDPPerm: AllowFillingExistingFormFieldsAndSignaturesPerms,
		},
		Signer:      key,
		Certificate: cert,
		Appearance: Appearance{
			SignerUID: uid,
		},
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	sf, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sf.Close() }()
	sfi, _ := sf.Stat()
	rdr, err := pdf.NewReader(sf, sfi.Size())
	if err != nil {
		t.Fatal(err)
	}

	fields := rdr.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	var checked int
	for i := 0; i < fields.Len(); i++ {
		field := fields.Index(i)
		decoded := decodeFieldNameFromPDF(field.Key("T").RawString())
		if !strings.Contains(decoded, "initials_page_") || !strings.Contains(decoded, uid) {
			continue
		}
		checked++
		ff := field.Key("Ff").Int64()
		if ff&2 == 0 {
			t.Fatalf("field %q missing read-only flag, Ff=%d", decoded, ff)
		}
		hasAP := !field.Key("AP").IsNull()
		if !hasAP {
			kids := field.Key("Kids")
			for k := 0; k < kids.Len(); k++ {
				if !kids.Index(k).Key("AP").IsNull() {
					hasAP = true
					break
				}
			}
		}
		if !hasAP {
			t.Fatalf("field %q missing /AP appearance on field or widget", decoded)
		}
	}
	if checked == 0 {
		t.Fatal("no initials fields checked")
	}
}

func TestSignPDFInitialsUTF16FieldName(t *testing.T) {
	// Ensure UTF-16 BOM field names decode consistently with fillable field logic.
	name := string([]byte{0xfe, 0xff})
	u16s := utf16.Encode([]rune("initials_page_1_signer_test"))
	for _, u := range u16s {
		name += string([]byte{byte(u >> 8), byte(u)})
	}
	decoded := decodeFieldName(name)
	if !strings.Contains(decoded, "initials_page_1_signer_test") {
		t.Fatalf("decode failed: %q", decoded)
	}
}
