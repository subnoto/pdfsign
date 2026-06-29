# PDF Parser for Go

A high-performance, lightweight PDF parsing library for [Go](https://go.dev), forked from `rsc/pdf`.

This library has been extensively refactored to support modern PDF standards and high-throughput production environments with a focus on memory efficiency and security.

## Key Improvements

### 1. High-Performance Zero-Allocation AST
The internal Abstract Syntax Tree (AST) has been rewritten to use a rigid `Object` union struct instead of `interface{}`. This eliminates the overhead of interface boxing for every PDF object (integers, names, strings, etc.), leading to massive reductions in memory allocations and GC pressure.

### 2. Modern Security Support
Added comprehensive support for encrypted PDFs:
- **AES-128 (v4)**: Full implementation of AES-CBC decryption for strings and streams.
- **AES-256 (v5)**: Support for PDF 2.0 / Extension Level 3 security handlers, including SHA-256 based Key Derivation (KDK) and File Encryption Key (FEK) retrieval.

### 3. Stability & Error Handling
- **Panic-Free Design**: Removed legacy `panic` calls in favor of proper Go error propagation.
- **Safe Method Chaining**: The `Value` struct now carries error state, allowing safe nested calls like `doc.Trailer().Key("Root").Key("Pages").Count()`.
- **Robustness**: Improved recovery from malformed PDF structures and strict parsing errors.

### 4. Memory Efficiency
- **Buffer Pooling**: Implemented `sync.Pool` for parsing buffers.
- **Bulk Scanning**: Optimized `lex.go` with specialized bulk scanners for Names, Keywords, and Strings, drastically reducing per-byte overhead.

## Benchmarks

Throughput comparison against the original library (parsing standard documents):

| Metric | Upstream Library | This Version | Change |
|--------|------------------|--------------|--------|
| **Parsing Speed** | 79,526 ns/op | 66,925 ns/op | **~16% Faster** |
| **Allocations** | 2,517 allocs/op | 97 allocs/op | **96% Reduction** |
| **Memory usage** | 113,712 B/op | 87,226 B/op | **23% Lower** |

## Usage

```go
import "github.com/digitorus/pdf"

r, err := pdf.NewReader(file, size)
if err != nil {
    return err
}

// Fluent, error-safe access
root := r.Trailer().Key("Root")
if err := root.Err(); err != nil {
    return err
}
```
