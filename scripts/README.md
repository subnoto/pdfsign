# Signed PDF conformance validation

Three layers validate outputs under `testfiles/success/` (created by `go test -v ./sign/...`).

| Layer | Tool | Standards |
| ----- | ---- | --------- |
| PDF structure + byte-range integrity | [pdfcpu](https://github.com/pdfcpu/pdfcpu) | ISO 32000 (strict mode) |
| Cryptographic verification | `pdfsign verify` / `verify` package | RFC 5652 (CMS), RFC 3161 (timestamps), RFC 9336 (cert profile) |
| PAdES structure + crypto (Go tests) | `sign/conformance_test.go` | ISO 32000 signature dictionaries, PAdES CMS detached profile |
| PAdES validation (optional) | [EU DSS](https://github.com/esig/dss) via REST | ETSI EN 319 142 (via DSS validator) |

## Quick start

```bash
# 1. Generate signed fixtures
go test -v ./sign/...

# 2. pdfcpu + pdfsign verify
./scripts/validate-signed.sh

# 3. Add EU DSS / ETSI PAdES checks (starts Docker automatically)
./scripts/validate-signed.sh --dss --with-dss-docker

# Or point at a running DSS instance
DSS_URL=http://localhost:8080 ./scripts/validate-signed.sh --dss
```

## Scripts

### `validate-signed.sh`

Main entry point. Defaults to all non-empty PDFs in `testfiles/success/`.

```bash
./scripts/validate-signed.sh [options] [file.pdf ...]

  --skip-pdfcpu        Skip pdfcpu
  --skip-verify        Skip pdfsign verify
  --dss                Run DSS validation
  --with-dss-docker    Start ghcr.io/vysmaty/dockerized-dss:latest
  --dss-url URL        DSS base URL (default: $DSS_URL)
```

### `dss_validate.py`

Calls `POST …/services/rest/validation/validateSignature`. For test certificates
(chain untrusted), pass criteria are **structural and cryptographic integrity**
(`SignatureValid`, `SignatureIntact`, `SignatureByteRange.valid`), not EU trust-list
qualification.

## Manual ETSI conformance checker

For full **ETSI TS 119 144** assertion reports (structure vs EN 319 142, separate
from trust-path validation):

1. Register at [ETSI Signature Conformance Checker](https://signatures-conformance-checker.etsi.org/)
2. Upload signed PDFs from `testfiles/success/`

This is interactive only (no public API); use it for release QA alongside automated DSS checks.

## Go tests

```bash
go test ./sign/ -run 'TestPAdES'
```

- `TestPAdESSignatureStructure` — signs a fixture and asserts dictionary fields
- `TestPAdESStructureOnSuccessFixtures` — walks `testfiles/success/` when present
