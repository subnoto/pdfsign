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

	t.Run("prefersLatestContentsPlaceholder", func(t *testing.T) {
		const maxLen = 8
		// Simulated prior signature with long zero padding in /Contents.
		prior := []byte("/Contents<3082010100")
		prior = append(prior, bytes.Repeat([]byte("0"), 32)...)
		prior = append(prior, ">\n"...)
		// New signature placeholder appended at end.
		newSig := []byte("<< /ByteRange[0 ********** ********** **********] /Contents<")
		newSig = append(newSig, bytes.Repeat([]byte("0"), maxLen)...)
		newSig = append(newSig, ">\n>>\n"...)

		ctx := &SignContext{
			SignatureMaxLength: maxLen,
			OutputBuffer:       filebuffer.New(append(prior, newSig...)),
		}
		if err := ctx.updateByteRange(); err != nil {
			t.Fatalf("updateByteRange: %v", err)
		}
		wantStart := int64(bytes.LastIndex(append(prior, newSig...), []byte("/Contents<")) + len("/Contents<") - 1)
		if ctx.ByteRangeValues[1] != wantStart {
			t.Fatalf("ByteRange[1] = %d, want %d (latest placeholder)", ctx.ByteRangeValues[1], wantStart)
		}
	})

	t.Run("lastByteRangePlaceholder", func(t *testing.T) {
		const maxLen = 4
		oldPlaceholder := []byte("<< /ByteRange[0 ********** ********** **********] /Contents<deadbeef>\n>>\n")
		newSig := []byte("<< /ByteRange[0 ********** ********** **********] /Contents<")
		newSig = append(newSig, bytes.Repeat([]byte("0"), maxLen)...)
		newSig = append(newSig, ">\n>>\n"...)

		content := append(oldPlaceholder, newSig...)
		ctx := &SignContext{
			SignatureMaxLength: maxLen,
			OutputBuffer:       filebuffer.New(content),
		}
		if err := ctx.updateByteRange(); err != nil {
			t.Fatalf("updateByteRange: %v", err)
		}
		out := ctx.OutputBuffer.Buff.Bytes()
		lastBR := bytes.LastIndex(out, []byte("/ByteRange ["))
		if lastBR == -1 {
			t.Fatal("expected replaced ByteRange in output")
		}
		if bytes.Contains(out[:lastBR], []byte("/ByteRange [")) {
			// Old placeholder should remain unchanged (still has asterisks).
			oldIdx := bytes.Index(out, []byte("/ByteRange[0 **********"))
			if oldIdx == -1 || oldIdx >= lastBR {
				t.Fatal("expected first ByteRange placeholder to remain as asterisk placeholder")
			}
		}
	})
}
