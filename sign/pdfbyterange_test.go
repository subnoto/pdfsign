package sign

import (
	"bytes"
	"testing"

	"github.com/mattetti/filebuffer"
)

func TestUpdateByteRange(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctx := &SignContext{
			SignatureMaxLength: 4,
			OutputBuffer:       filebuffer.New(nil),
		}
		content := []byte("<< /ByteRange[0 ********** ********** **********] /Contents<0000>\n>>\n")
		if _, err := ctx.OutputBuffer.Write(content); err != nil {
			t.Fatal(err)
		}

		if err := ctx.updateByteRange(); err != nil {
			t.Fatalf("updateByteRange: %v", err)
		}

		out := ctx.OutputBuffer.Buff.Bytes()
		if !bytes.Contains(out, []byte("/ByteRange [")) {
			t.Fatalf("expected real ByteRange, got:\n%s", out)
		}
		if ctx.ByteRangeValues[0] != 0 {
			t.Fatalf("ByteRange[0] = %d, want 0", ctx.ByteRangeValues[0])
		}
	})

	t.Run("missing contents placeholder", func(t *testing.T) {
		ctx := &SignContext{
			SignatureMaxLength: 8,
			OutputBuffer:       filebuffer.New([]byte("no placeholder here\n")),
		}
		if err := ctx.updateByteRange(); err == nil {
			t.Fatal("expected error for missing contents placeholder")
		}
	})

	t.Run("missing byte range placeholder", func(t *testing.T) {
		ctx := &SignContext{
			SignatureMaxLength: 4,
			OutputBuffer:       filebuffer.New([]byte("/Contents<0000>\n")),
		}
		if err := ctx.updateByteRange(); err == nil {
			t.Fatal("expected error for missing ByteRange placeholder")
		}
	})
}
