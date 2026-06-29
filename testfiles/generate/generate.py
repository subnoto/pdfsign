#!/usr/bin/env python3
"""Generate committed PDF test fixtures using ReportLab and pikepdf (qpdf).

Regenerate from repo root:
  pip install -r testfiles/generate/requirements.txt
  python3 testfiles/generate/generate.py

PDFs are written to testfiles/ and should be committed to git.
"""

from __future__ import annotations

import sys
from pathlib import Path

import pikepdf
from pikepdf import Dictionary, Name, ObjectStreamMode
from reportlab.lib.pagesizes import A4, landscape
from reportlab.lib.units import inch
from reportlab.pdfbase.acroform import AcroForm
from reportlab.pdfgen import canvas

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT


def _save_variant(
    src: Path,
    dest: Path,
    *,
    version: str,
    xref_stream: bool,
) -> None:
    with pikepdf.open(src) as pdf:
        pdf.save(
            dest,
            force_version=version,
            object_stream_mode=(
                ObjectStreamMode.generate if xref_stream else ObjectStreamMode.disable
            ),
            compress_streams=True,
            normalize_content=True,
        )


def _write_single_page(path: Path, text: str, pagesize=A4) -> None:
    c = canvas.Canvas(str(path), pagesize=pagesize)
    c.setFont("Helvetica", 14)
    width, height = pagesize
    c.drawString(72, height - 72, text)
    c.showPage()
    c.save()


def _write_multipage(path: Path, labels: list[str]) -> None:
    c = canvas.Canvas(str(path))
    for label in labels:
        c.setFont("Helvetica", 14)
        c.drawString(72, 720, label)
        c.showPage()
    c.save()


def _wire_acroform_fields(pdf: pikepdf.Pdf) -> None:
    """Link page widget annotations into /AcroForm /Fields (ReportLab omits this)."""
    fields: list = []
    pages = pdf.Root.Pages.Kids
    for page in pages:
        for annot in page.get("/Annots", []):
            if annot.get("/Subtype") == Name("/Widget"):
                fields.append(annot)
    if not fields:
        return
    pdf.Root.AcroForm = pdf.make_indirect(
        Dictionary(
            Fields=pikepdf.Array(fields),
            DA="(/Helv 10 Tf 0 0 0 rg)",
        )
    )


def _write_acroform(path: Path) -> None:
    c = canvas.Canvas(str(path), pagesize=A4)
    c.setFont("Helvetica", 12)
    c.drawString(72, 700, "AcroForm PDF 1.4")
    form = AcroForm(c)
    form.textfield(
        name="initials_page_1_signer_test",
        tooltip="Initials",
        x=72,
        y=650,
        width=128,
        height=30,
        borderStyle="inset",
        borderWidth=1,
        forceBorder=True,
    )
    c.showPage()
    c.save()


def _save_acroform_variant(src: Path, dest: Path, *, version: str) -> None:
    with pikepdf.open(src) as pdf:
        _wire_acroform_fields(pdf)
        pdf.save(
            dest,
            force_version=version,
            object_stream_mode=ObjectStreamMode.disable,
            compress_streams=True,
            normalize_content=True,
        )


def _write_nested_page_tree(src: Path, dest: Path, version: str) -> None:
    with pikepdf.open(src) as pdf:
        root = pdf.Root
        pages = root.Pages
        leaf = pages.Kids[0]

        intermediate = pdf.make_indirect(
            Dictionary(
                Type=Name.Pages,
                Count=1,
                Kids=pikepdf.Array([leaf]),
            )
        )
        leaf.Parent = intermediate

        root_pages = pdf.make_indirect(
            Dictionary(
                Type=Name.Pages,
                Count=1,
                Kids=pikepdf.Array([intermediate]),
            )
        )
        root.Pages = root_pages

        pdf.save(
            dest,
            force_version=version,
            object_stream_mode=ObjectStreamMode.generate,
            compress_streams=True,
            normalize_content=True,
        )


def main() -> int:
    tmp = OUT / ".generate_tmp"
    tmp.mkdir(exist_ok=True)

    singles = {
        "single_portrait.pdf": ("PDF base portrait", A4),
        "single_landscape.pdf": ("Landscape A4", landscape(A4)),
    }
    for name, (text, size) in singles.items():
        _write_single_page(tmp / name, text, size)

    _write_multipage(tmp / "three_pages.pdf", ["Page 1 of 3", "Page 2 of 3", "Page 3 of 3"])
    _write_acroform(tmp / "acroform.pdf")

    fixtures = [
        ("gen_pdf13_xref_table.pdf", tmp / "single_portrait.pdf", "1.3", False),
        ("gen_pdf14_acroform.pdf", tmp / "acroform.pdf", "1.4", False),
        ("gen_pdf15_xref_stream.pdf", tmp / "single_portrait.pdf", "1.5", True),
        ("gen_pdf16_xref_table_3pages.pdf", tmp / "three_pages.pdf", "1.6", False),
        ("gen_pdf17_xref_stream_landscape.pdf", tmp / "single_landscape.pdf", "1.7", True),
        ("gen_pdf20_xref_stream.pdf", tmp / "single_portrait.pdf", "2.0", True),
    ]

    for dest_name, src, version, xref_stream in fixtures:
        dest = OUT / dest_name
        if dest_name == "gen_pdf14_acroform.pdf":
            _save_acroform_variant(src, dest, version=version)
        else:
            _save_variant(src, dest, version=version, xref_stream=xref_stream)
        print(f"wrote {dest} ({dest.stat().st_size} bytes)")

    nested_dest = OUT / "gen_pdf17_nested_page_tree.pdf"
    _write_nested_page_tree(tmp / "single_portrait.pdf", nested_dest, "1.7")
    print(f"wrote {nested_dest} ({nested_dest.stat().st_size} bytes)")

    for f in tmp.glob("*.pdf"):
        f.unlink()
    tmp.rmdir()

    return 0


if __name__ == "__main__":
    sys.exit(main())
