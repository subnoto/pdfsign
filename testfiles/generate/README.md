# Test PDF fixtures

The `gen_*.pdf` files in the parent directory are **committed to the repository**
and exercised by `sign/gen_fixtures_test.go` and `TestSignPDF`.

They are produced with well-known tools, not hand-written PDF bytes:

- **[ReportLab](https://www.reportlab.com/)** — page content, multi-page documents, AcroForm fields
- **[pikepdf](https://pikepdf.readthedocs.io/)** (qpdf) — PDF version, classic xref table vs cross-reference stream

## Regenerate

```bash
./testfiles/generate/generate.sh
```

Or manually:

```bash
python3 -m venv testfiles/generate/.venv
source testfiles/generate/.venv/bin/activate
pip install -r testfiles/generate/requirements.txt
python3 testfiles/generate/generate.py
```

## Fixtures

| File | PDF version | Xref |
|------|-------------|------|
| `gen_pdf13_xref_table.pdf` | 1.3 | table |
| `gen_pdf14_acroform.pdf` | 1.4 | table + AcroForm |
| `gen_pdf15_xref_stream.pdf` | 1.5 | stream |
| `gen_pdf16_xref_table_3pages.pdf` | 1.6 | table, 3 pages |
| `gen_pdf17_xref_stream_landscape.pdf` | 1.7 | stream, landscape |
| `gen_pdf17_nested_page_tree.pdf` | 1.7 | stream, nested `/Pages` |
| `gen_pdf20_xref_stream.pdf` | 2.0 | stream |
