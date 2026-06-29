package pdf

import (
	"testing"
)

func TestIsPDFDocEncoded(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Hello", true},
		{"\xfe\xff\x00H\x00e\x00l\x00l\x00o", false}, // UTF-16BE
		{"\x00", false}, // pdfDocEncoding[0] is noRune
	}

	for _, tt := range tests {
		if got := isPDFDocEncoded(tt.input); got != tt.want {
			t.Errorf("isPDFDocEncoded(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestPDFDocDecode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "Hello"},
		{"\x1a", "\u02c6"}, // circumflex
		{"\x1c", "\u02dd"}, // double acute / hungarumlaut
		{"\x18\x19\x1a", "\u02d8\u02c7\u02c6"},
	}

	for _, tt := range tests {
		if got := pdfDocDecode(tt.input); got != tt.want {
			t.Errorf("pdfDocDecode(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsUTF16(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"\xfe\xff", true},
		{"\xfe\xff\x00A", true},
		{"\xfe", false},
		{"\xff\xfe", false},
		{"Hello", false},
		{"\xfe\xff\x00", false}, // odd length
	}

	for _, tt := range tests {
		if got := isUTF16(tt.input); got != tt.want {
			t.Errorf("isUTF16(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestUTF16Decode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\x00H\x00e\x00l\x00l\x00o", "Hello"},
		{"\x00A", "A"},
	}

	for _, tt := range tests {
		if got := utf16Decode(tt.input); got != tt.want {
			t.Errorf("utf16Decode(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}
