package pdf

import (
	"bytes"
	"io"
	"testing"
)

func TestASCIIHexDecode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"414243>", "ABC"},
		{"61 62 63 >", "abc"},
		{"414>", "A@"}, // Odd length assumes 0 trailing
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := bytes.NewReader([]byte(tt.input))
			// applyFilter(rd io.Reader, name string, param Value)
			gotReader, err := applyFilter(r, "ASCIIHexDecode", Value{})
			if err != nil {
				t.Fatalf("applyFilter failed: %v", err)
			}

			got, _ := io.ReadAll(gotReader)
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestASCII85Decode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"87cUR", "Hell"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := bytes.NewReader([]byte(tt.input))
			gotReader, err := applyFilter(r, "ASCII85Decode", Value{})
			if err != nil {
				t.Fatalf("applyFilter failed: %v", err)
			}

			got, err := io.ReadAll(gotReader)
			if err != nil && err != io.EOF {
				t.Logf("Read returned error: %v (data: %q)", err, string(got))
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestFlateDecode(t *testing.T) {
	// zlib compressed "Hello World"
	input := []byte{0x78, 0x9c, 0xf2, 0x48, 0xcd, 0xc9, 0xc9, 0x57, 0x08, 0xcf, 0x2f, 0xca, 0x49, 0x01, 0x04, 0x00, 0x00, 0xff, 0xff, 0x1a, 0x0b, 0x04, 0x5d}
	want := "Hello World"

	r := bytes.NewReader(input)
	gotReader, err := applyFilter(r, "FlateDecode", Value{})
	if err != nil {
		t.Fatalf("applyFilter failed: %v", err)
	}

	got, _ := io.ReadAll(gotReader)
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}
