#!/usr/bin/env python3
"""
Validate signed PDFs with the EU Digital Signature Service (DSS) REST API.

DSS applies ETSI PAdES validation rules (EN 319 142). Test fixtures use a
self-signed certificate, so chain trust checks fail — we require structural
and cryptographic integrity instead:

  - StructuralValidation.valid
  - BasicSignature.SignatureIntact / SignatureValid
  - DigestMatcher DataIntact
  - PDF SignatureByteRange.valid

LTV/LTA fixtures (*_TestSignLTV.pdf, *_TestSignLTA.pdf) additionally require
ISO 32000 markers in the PDF bytes (/Type /DSS, /VRI, and for LTA /DocTimeStamp).

Usage:
  DSS_URL=http://localhost:8080 ./scripts/dss_validate.py file.pdf ...
  ./scripts/dss_validate.py --expect-ltv file.pdf
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
import urllib.error
import urllib.request
from pathlib import Path


def dss_base_url(raw: str) -> str:
    raw = raw.rstrip("/")
    if raw.endswith("/services/rest/validation"):
        return raw
    return raw + "/services/rest/validation"


def pdf_markers(data: bytes, markers: list[str]) -> list[str]:
    text = data.decode("latin-1", errors="ignore")
    return [m for m in markers if m not in text]


def infer_profile(path: Path) -> str:
    name = path.name
    if "_TestSignLTA" in name:
        return "lta"
    if "_TestSignLTV" in name:
        return "ltv"
    return "baseline"


def validate_pdf(
    path: Path,
    url: str,
    timeout: float,
    profile: str,
) -> tuple[bool, str]:
    data = path.read_bytes()
    if profile in ("ltv", "lta"):
        ltv_missing = pdf_markers(data, ["/Type /DSS", "/VRI"])
        if ltv_missing:
            return False, f"LTV markers missing: {ltv_missing}"
    if profile == "lta":
        lta_missing = pdf_markers(data, ["/Type /DocTimeStamp", "/SubFilter /ETSI.RFC3161"])
        if lta_missing:
            return False, f"LTA markers missing: {lta_missing}"

    payload = {
        "signedDocument": {
            "bytes": base64.b64encode(data).decode(),
            "name": path.name,
        },
        "tokenExtractionStrategy": "NONE",
    }
    req = urllib.request.Request(
        url + "/validateSignature",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", "Accept": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = json.loads(resp.read())
    except urllib.error.HTTPError as e:
        detail = e.read().decode(errors="replace")
        return False, f"HTTP {e.code}: {detail[:500]}"
    except urllib.error.URLError as e:
        return False, f"request failed: {e.reason}"

    sigs = body.get("DiagnosticData", {}).get("Signature") or []
    if not sigs:
        simple = body.get("SimpleReport", {})
        count = simple.get("SignaturesCount", 0)
        return False, f"no DiagnosticData.Signature entries (SignaturesCount={count})"

    problems: list[str] = []
    for i, sig in enumerate(sigs):
        prefix = f"signature[{i}]"
        structural = sig.get("StructuralValidation") or {}
        if structural.get("valid") is not True:
            msgs = structural.get("Message") or []
            problems.append(f"{prefix}: StructuralValidation.valid is false ({msgs})")

        basic = sig.get("BasicSignature") or {}
        if basic.get("SignatureIntact") is not True:
            problems.append(f"{prefix}: SignatureIntact is not true")
        if basic.get("SignatureValid") is not True:
            problems.append(f"{prefix}: SignatureValid is not true")

        for j, dm in enumerate(sig.get("DigestMatcher") or []):
            if dm.get("DataIntact") is not True:
                problems.append(f"{prefix}.DigestMatcher[{j}]: DataIntact is not true")

        pdf_rev = sig.get("PDFRevision") or {}
        pdf_sig = pdf_rev.get("PDFSignatureDictionary") or {}
        br = pdf_sig.get("SignatureByteRange") or {}
        if br and br.get("valid") is not True:
            problems.append(f"{prefix}: SignatureByteRange.valid is not true")

        sub = pdf_sig.get("SubFilter")
        if sub and sub not in ("adbe.pkcs7.detached", "ETSI.RFC3161"):
            problems.append(f"{prefix}: unexpected SubFilter {sub!r}")

    if problems:
        return False, "; ".join(problems)

    fmt = sigs[0].get("SignatureFormat", "?")
    sub = (sigs[0].get("PDFRevision") or {}).get("PDFSignatureDictionary", {}).get("SubFilter", "?")
    profile_label = {"baseline": "B-B/B-T", "ltv": "B-LT", "lta": "B-LTA"}.get(profile, profile)
    return True, f"ok ({len(sigs)} signature(s), profile={profile_label}, format={fmt}, subFilter={sub})"


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate signed PDFs with EU DSS")
    parser.add_argument("pdfs", nargs="+", type=Path, help="Signed PDF file(s)")
    parser.add_argument(
        "--url",
        default=None,
        help="DSS base URL (default: $DSS_URL or http://localhost:8080)",
    )
    parser.add_argument(
        "--profile",
        choices=("baseline", "ltv", "lta", "auto"),
        default="auto",
        help="Expected PAdES profile (default: infer from filename)",
    )
    parser.add_argument("--timeout", type=float, default=120.0, help="HTTP timeout seconds")
    args = parser.parse_args()

    import os

    base = args.url or os.environ.get("DSS_URL") or "http://localhost:8080"
    url = dss_base_url(base)

    failed = 0
    for pdf in args.pdfs:
        if not pdf.is_file():
            print(f"❌ {pdf}: not found", file=sys.stderr)
            failed += 1
            continue
        if pdf.stat().st_size == 0:
            print(f"❌ {pdf}: empty file", file=sys.stderr)
            failed += 1
            continue
        profile = infer_profile(pdf) if args.profile == "auto" else args.profile
        ok, msg = validate_pdf(pdf, url, args.timeout, profile)
        if ok:
            print(f"✅ DSS {pdf.name}: {msg}")
        else:
            print(f"❌ DSS {pdf.name}: {msg}", file=sys.stderr)
            failed += 1

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
