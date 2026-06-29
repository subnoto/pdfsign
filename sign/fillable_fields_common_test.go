package sign

import (
	"encoding/hex"
	"testing"
)

func TestNormalizeDA(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"multiline", "0 0 0 rg\n/Helvetica 12 Tf", "/F1 12 Tf 0 0 0 rg"},
		{"default size", "0 0 0 rg", "/F1 10 Tf 0 0 0 rg"},
		{"custom font size", "/Helvetica 14 Tf 0 0 0 rg", "/F1 14 Tf 0 0 0 rg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDA(tc.in); got != tc.want {
				t.Fatalf("normalizeDA(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDecodeFieldName(t *testing.T) {
	plain := decodeFieldName("initials_page_1_signer_abc")
	if plain != "initials_page_1_signer_abc" {
		t.Fatalf("plain name: got %q", plain)
	}

	utf16BE := string([]byte{0xfe, 0xff, 0x00, 0x61, 0x00, 0x62})
	if got := decodeFieldName(utf16BE); got != "ab" {
		t.Fatalf("UTF-16 BE: got %q", got)
	}

	utf16LE := string([]byte{0xff, 0xfe, 0x61, 0x00, 0x62, 0x00})
	if got := decodeFieldName(utf16LE); got != "ab" {
		t.Fatalf("UTF-16 LE: got %q", got)
	}
}

func TestMatchFieldSigner(t *testing.T) {
	pattern := `^initials_page_(\d+)_signer_(.+)$`
	uid := "toto@toto.com"
	hexUID := hex.EncodeToString([]byte(uid))

	tests := []struct {
		name      string
		fieldName string
		want      bool
	}{
		{"plain uid", "initials_page_1_signer_toto@toto.com", true},
		{"hex uid", "initials_page_1_signer_" + hexUID, true},
		{"wrong uid", "initials_page_1_signer_other@example.com", false},
		{"wrong prefix", "date_id_1_signer_" + hexUID, false},
		{"fallback hex tail", "initials_page_1_signer_" + hexUID, true},
		{"extra suffix after hex", "initials_page_1_signer_" + hexUID + "_extra", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched, _ := matchFieldSigner(tc.fieldName, pattern, uid)
			if matched != tc.want {
				t.Fatalf("matchFieldSigner(%q) = %v, want %v", tc.fieldName, matched, tc.want)
			}
		})
	}

	// hex-decoded field signer value
	fieldSignerHex := hex.EncodeToString([]byte("user@example.com"))
	matched, signer := matchFieldSigner("initials_page_2_signer_"+fieldSignerHex, pattern, "user@example.com")
	if !matched || signer != fieldSignerHex {
		t.Fatalf("hex-decoded match failed: matched=%v signer=%q", matched, signer)
	}
}
