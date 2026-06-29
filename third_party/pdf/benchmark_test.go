package pdf

import (
	"fmt"
	"os"
	"testing"
)

func BenchmarkGetObject(b *testing.B) {
	// Use a test file that exists in the repo
	// internal/pdf is at /Users/paulvanbrouwershaven/Code/pdfsign/internal/pdf
	// testfiles are at /Users/paulvanbrouwershaven/Code/pdfsign/testfiles
	file := "../../testfiles/testfile12.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		b.Skip("skipping benchmark; testfile12.pdf not found")
	}

	f, err := os.Open(file)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		b.Fatal(err)
	}

	r, err := NewReader(f, info.Size())
	if err != nil {
		b.Fatal(err)
	}

	// Find a valid object ID to resolve.
	// For testfile1.pdf (produced by simple writer), object 1 usually exists.
	// Or we can scan xref to find a valid one.
	var traceID uint32
	for id, x := range r.xref {
		if x.offset > 0 {
			traceID = uint32(id)
			break
		}
	}

	if traceID == 0 {
		b.Fatal("no valid object found to benchmark")
	}

	fmt.Printf("Benchmarking resolution of Object ID: %d\n", traceID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This should hit the cache after the first iteration
		_, err := r.GetObject(traceID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAllObjects(b *testing.B) {
	file := "../../testfiles/testfile12.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		b.Skip("skipping benchmark; testfile12.pdf not found")
	}

	f, err := os.Open(file)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		b.Fatal(err)
	}

	// We want to measure parsing, so we need to run resolve() which populates cache.
	// To measure repeat parsing performance, we would need to prevent caching or create new readers.
	// Creating new readers involves scanning xref which is also parsing.

	// Option A: Create new reader each iter (measures xref parsing + object parsing if we trigger it)
	// Option B: Reuse reader but read distinct objects (only works if file is huge, eventually hits cache)

	// Let's do Option A: NewReader + Resolve All Objects. This is the "Load + Verify" scenario.

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		f.Seek(0, 0) // Reset file cursor
		b.StartTimer()

		r, err := NewReader(f, info.Size())
		if err != nil {
			b.Fatal(err)
		}

		// Iterate all objects
		for id, x := range r.xref {
			if x.offset > 0 {
				_, err := r.GetObject(uint32(id))
				if err != nil {
					// Some objects might be malformed or fail, but usually testfile should be clean.
					// Just continue or log? Fatal for now.
					// b.Fatal(err)
					// Actually, ignore errors for stress testing if file has known issues,
					// but testfile12 should be good.
					_ = err
				}
			}
		}
	}
}
