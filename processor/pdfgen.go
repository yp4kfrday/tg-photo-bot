// Package processor — PDF generation via a dense center-crop grid.
//
// Layout (A4, portrait):
//
//	┌─────────────────────────────────────────┐  ↑ 3 mm margin
//	│ [49.9mm] 1.5 [49.9mm] 1.5 [49.9mm] 1.5 [49.9mm] │
//	│ ─────────────────────────── 1.5 mm gap ────────── │
//	│ [49.9mm]     [49.9mm]      [49.9mm]     [49.9mm]  │  × 5 rows
//	│  ...                                               │
//	└─────────────────────────────────────────┘  ↓ 3 mm margin
//
// Each cell is filled with a center-cropped photo (imaging.Fill),
// so there are no white borders inside cells regardless of the original
// photo proportions.
package processor

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"log/slog"
	"math"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/go-pdf/fpdf"
)

// ── Grid constants ────────────────────────────────────────────────────────────

const (
	// Page size (mm).
	pdfPageW = 210.0
	pdfPageH = 297.0

	// Minimal margins — just enough so content isn't clipped by the printer.
	pdfMargin = 3.0

	// Gaps between cells (mm).
	pdfColGap = 1.5
	pdfRowGap = 1.5

	// Grid dimensions.
	pdfCols = 4
	pdfRows = 5

	// Target print resolution (DPI) for rasterised cells.
	// 150 DPI gives good print quality without oversized files.
	pdfDPI = 150.0
)

// Derived dimensions computed once at package initialisation.
var (
	// Cell width/height in millimetres.
	//   cellW = (usableW − (cols−1)×colGap) / cols
	//         = (204 − 3×1.5) / 4 = 49.875 mm
	pdfCellW = (pdfPageW - 2*pdfMargin - float64(pdfCols-1)*pdfColGap) / float64(pdfCols)

	//   cellH = (usableH − (rows−1)×rowGap) / rows
	//         = (291 − 4×1.5) / 5 = 57.0 mm
	pdfCellH = (pdfPageH - 2*pdfMargin - float64(pdfRows-1)*pdfRowGap) / float64(pdfRows)

	// Cell size in pixels at pdfDPI.
	// px = mm × (DPI / 25.4)
	pdfCellPxW = int(math.Round(pdfCellW * pdfDPI / 25.4)) // ≈ 295 px
	pdfCellPxH = int(math.Round(pdfCellH * pdfDPI / 25.4)) // ≈ 337 px

	// Pre-computed X origin for each column (mm from left edge).
	pdfColX = func() [pdfCols]float64 {
		var x [pdfCols]float64
		for i := 0; i < pdfCols; i++ {
			x[i] = pdfMargin + float64(i)*(pdfCellW+pdfColGap)
		}
		return x
	}()

	// Pre-computed Y origin for each row on a fresh page (mm from top edge).
	pdfRowY = func() [pdfRows]float64 {
		var y [pdfRows]float64
		for i := 0; i < pdfRows; i++ {
			y[i] = pdfMargin + float64(i)*(pdfCellH+pdfRowGap)
		}
		return y
	}()
)

// perPage is the maximum number of photos on a single page.
const perPage = pdfCols * pdfRows // 4 × 5 = 20

// GeneratePDF builds a dense A4 PDF collage from the given JPEG/PNG source paths.
//
// Each photo is center-cropped and resized to a uniform cell (pdfCellPxW × pdfCellPxH px)
// using imaging.Fill so that every cell is fully covered with no white padding.
// Cropped intermediates are written inside tempDir and cleaned up by the caller
// (processor.Process already defers os.RemoveAll on tempDir).
//
// If there are more than perPage (20) photos, additional A4 pages are added
// automatically.
func GeneratePDF(srcPaths []string, tempDir, outPath string) error {
	if len(srcPaths) == 0 {
		return fmt.Errorf("no photos to render")
	}

	slog.Info("PDF layout",
		"cols", pdfCols, "rows", pdfRows,
		"cellW_mm", fmt.Sprintf("%.2f", pdfCellW),
		"cellH_mm", fmt.Sprintf("%.2f", pdfCellH),
		"cellPx", fmt.Sprintf("%d×%d", pdfCellPxW, pdfCellPxH),
		"perPage", perPage,
	)

	// ── Phase 1: center-crop every photo to exact cell dimensions ────────────
	type cell struct {
		path string // path to the cropped JPEG ready for fpdf.Image
	}
	cells := make([]cell, 0, len(srcPaths))

	for i, src := range srcPaths {
		dst := filepath.Join(tempDir, fmt.Sprintf("cell_%03d.jpg", i+1))
		if err := prepareCell(src, dst); err != nil {
			return fmt.Errorf("prepare cell[%d] from %q: %w", i+1, src, err)
		}
		cells = append(cells, cell{path: dst})
		slog.Debug("cell prepared", "index", i+1, "src", src)
	}

	// ── Phase 2: build the PDF ───────────────────────────────────────────────
	// "" for fontDirStr means only built-in fonts — we don't use text at all.
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(false, pdfMargin)

	pageCount := 0

	for i, c := range cells {
		posOnPage := i % perPage
		col := posOnPage % pdfCols
		row := posOnPage / pdfCols

		// Start a new page at the beginning of each full grid.
		if posOnPage == 0 {
			pdf.AddPage()
			pageCount++
		}

		x := pdfColX[col]
		y := pdfRowY[row]

		// Place the pre-cropped JPEG at exact cell dimensions.
		// "JPEG" is explicit — avoids extension-based auto-detection overhead.
		pdf.Image(c.path, x, y, pdfCellW, pdfCellH, false, "JPEG", 0, "")
	}

	if err := pdf.OutputFileAndClose(outPath); err != nil {
		return fmt.Errorf("write PDF %q: %w", outPath, err)
	}

	slog.Info("PDF created",
		"path", outPath,
		"pages", pageCount,
		"photos", len(cells),
	)
	return nil
}

// prepareCell opens srcPath (JPEG or PNG), applies a center-crop-and-resize to
// exactly pdfCellPxW × pdfCellPxH pixels using Lanczos resampling, then saves
// the result as a JPEG (quality 85) to dstPath.
//
// imaging.Fill resizes the image so that it fully covers the target rectangle
// and then crops the excess from the centre — no white bars, no distortion.
func prepareCell(srcPath, dstPath string) error {
	// imaging.Open decodes JPEG, PNG, GIF, BMP, TIFF automatically.
	src, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("open %q: %w", srcPath, err)
	}

	// Fill = scale-to-cover + center-crop → exact pdfCellPxW × pdfCellPxH.
	cropped := imaging.Fill(src, pdfCellPxW, pdfCellPxH, imaging.Center, imaging.Lanczos)

	// Save as JPEG at quality 85. imaging infers format from the .jpg extension.
	if err = imaging.Save(cropped, dstPath, imaging.JPEGQuality(85)); err != nil {
		return fmt.Errorf("save %q: %w", dstPath, err)
	}
	return nil
}

// dimensions returns the pixel width and height of an image file without
// decoding the full pixel data (reads only the header).
func dimensions(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
