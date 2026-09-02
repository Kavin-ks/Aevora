# Aevora — Mini-Project Report (LaTeX)

## Files

| File | Purpose |
|---|---|
| `aevora-report.tex` | Main LaTeX source (complete, self-contained) |
| `aevora-report.bib` | BibTeX bibliography (IEEE style) |

## Compile to PDF

### Option 1 — Overleaf (recommended, no software needed)

1. Go to [overleaf.com](https://www.overleaf.com) → New Project → Upload Project
2. Upload both `aevora-report.tex` and `aevora-report.bib`
3. Set compiler to **pdfLaTeX** (default)
4. Click **Recompile** — PDF appears in seconds

### Option 2 — Local (macOS with MacTeX)

```bash
brew install --cask mactex   # ~4 GB, one-time
cd docs/report
pdflatex aevora-report.tex
bibtex aevora-report
pdflatex aevora-report.tex
pdflatex aevora-report.tex   # third pass for TOC/refs
open aevora-report.pdf
```

### Option 3 — Docker (no installation)

```bash
docker run --rm \
  -v "$(pwd)/docs/report:/data" \
  texlive/texlive:latest \
  bash -c "cd /data && \
    pdflatex -interaction=nonstopmode aevora-report.tex && \
    bibtex aevora-report && \
    pdflatex -interaction=nonstopmode aevora-report.tex && \
    pdflatex -interaction=nonstopmode aevora-report.tex"
open docs/report/aevora-report.pdf
```

> **Note**: Run pdflatex three times (once for content, once after bibtex, once to resolve cross-references in TOC/figures).
