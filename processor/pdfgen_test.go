package processor

import (
	"archive/zip"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// makeTestJPEG writes a gradient JPEG of size w×h to path.
func makeTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: 128,
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("makeTestJPEG create %s: %v", path, err)
	}
	defer f.Close()
	if err = jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("makeTestJPEG encode %s: %v", path, err)
	}
}

func mustNoErr(t *testing.T, err error, label string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

// imgDims reads only the JPEG/PNG header to return pixel dimensions.
func imgDims(t *testing.T, path string) (w, h int) {
	t.Helper()
	w, h, err := dimensions(path)
	mustNoErr(t, err, "dimensions "+path)
	return w, h
}

// ─────────────────────────────────────────────────────────────────────────────
// Pack (archiver) tests
// ─────────────────────────────────────────────────────────────────────────────

func TestPack_CreatesValidZip(t *testing.T) {
	dir := t.TempDir()
	names := []string{"photo_001.jpg", "photo_002.jpg", "photo_003.jpg"}
	var srcs []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		makeTestJPEG(t, p, 320, 240)
		srcs = append(srcs, p)
	}

	zipPath := filepath.Join(dir, "out.zip")
	mustNoErr(t, Pack(srcs, zipPath), "Pack")

	r, err := zip.OpenReader(zipPath)
	mustNoErr(t, err, "zip.OpenReader")
	defer r.Close()

	if got, want := len(r.File), len(names); got != want {
		t.Errorf("zip has %d entries, want %d", got, want)
	}
	index := map[string]bool{}
	for _, f := range r.File {
		index[f.Name] = true
	}
	for _, n := range names {
		if !index[n] {
			t.Errorf("zip missing entry %q", n)
		}
	}
}

func TestPack_NilInput_ProducesEmptyZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "empty.zip")
	mustNoErr(t, Pack(nil, zipPath), "Pack(nil)")

	r, err := zip.OpenReader(zipPath)
	mustNoErr(t, err, "zip.OpenReader")
	defer r.Close()
	if n := len(r.File); n != 0 {
		t.Errorf("expected 0 entries, got %d", n)
	}
}

func TestPack_NoPathTraversal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "photo_001.jpg")
	makeTestJPEG(t, p, 100, 100)

	zipPath := filepath.Join(dir, "out.zip")
	mustNoErr(t, Pack([]string{p}, zipPath), "Pack")

	r, err := zip.OpenReader(zipPath)
	mustNoErr(t, err, "zip.OpenReader")
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) != f.Name {
			t.Errorf("entry %q contains directory component (traversal risk)", f.Name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// prepareCell tests
// ─────────────────────────────────────────────────────────────────────────────

// TestPrepareCell_OutputDimensions verifies that the output is exactly
// pdfCellPxW × pdfCellPxH regardless of the input aspect ratio.
func TestPrepareCell_OutputDimensions_Landscape(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "landscape.jpg")
	makeTestJPEG(t, src, 1200, 900) // 4:3 landscape → will be cropped to portrait cell

	dst := filepath.Join(dir, "cell.jpg")
	mustNoErr(t, prepareCell(src, dst), "prepareCell")

	w, h := imgDims(t, dst)
	if w != pdfCellPxW || h != pdfCellPxH {
		t.Errorf("dimensions = %d×%d, want %d×%d", w, h, pdfCellPxW, pdfCellPxH)
	}
}

func TestPrepareCell_OutputDimensions_Portrait(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "portrait.jpg")
	makeTestJPEG(t, src, 600, 1200) // tall portrait

	dst := filepath.Join(dir, "cell.jpg")
	mustNoErr(t, prepareCell(src, dst), "prepareCell")

	w, h := imgDims(t, dst)
	if w != pdfCellPxW || h != pdfCellPxH {
		t.Errorf("dimensions = %d×%d, want %d×%d", w, h, pdfCellPxW, pdfCellPxH)
	}
}

func TestPrepareCell_OutputDimensions_Square(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "square.jpg")
	makeTestJPEG(t, src, 800, 800)

	dst := filepath.Join(dir, "cell.jpg")
	mustNoErr(t, prepareCell(src, dst), "prepareCell")

	w, h := imgDims(t, dst)
	if w != pdfCellPxW || h != pdfCellPxH {
		t.Errorf("dimensions = %d×%d, want %d×%d", w, h, pdfCellPxW, pdfCellPxH)
	}
}

func TestPrepareCell_WritesValidJPEG(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	makeTestJPEG(t, src, 400, 300)

	dst := filepath.Join(dir, "cell.jpg")
	mustNoErr(t, prepareCell(src, dst), "prepareCell")

	f, err := os.Open(dst)
	mustNoErr(t, err, "open dst")
	defer f.Close()

	_, format, err := image.Decode(f)
	mustNoErr(t, err, "image.Decode")
	if format != "jpeg" {
		t.Errorf("output format = %q, want jpeg", format)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GeneratePDF tests
// ─────────────────────────────────────────────────────────────────────────────

func TestGeneratePDF_SinglePhoto(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo_001.jpg")
	makeTestJPEG(t, src, 800, 600)

	out := filepath.Join(dir, "out.pdf")
	mustNoErr(t, GeneratePDF([]string{src}, dir, out), "GeneratePDF")

	info, err := os.Stat(out)
	mustNoErr(t, err, "stat PDF")
	if info.Size() == 0 {
		t.Error("generated PDF is empty")
	}
}

func TestGeneratePDF_ExactlyOnePage(t *testing.T) {
	dir := t.TempDir()
	// perPage = 20; fill exactly one page.
	var srcs []string
	for i := 0; i < perPage; i++ {
		p := filepath.Join(dir, fmt.Sprintf("photo_%03d.jpg", i+1))
		makeTestJPEG(t, p, 600, 800)
		srcs = append(srcs, p)
	}

	out := filepath.Join(dir, "out.pdf")
	mustNoErr(t, GeneratePDF(srcs, dir, out), "GeneratePDF")

	info, err := os.Stat(out)
	mustNoErr(t, err, "stat PDF")
	if info.Size() < 2048 {
		t.Errorf("PDF suspiciously small (%d bytes)", info.Size())
	}
}

func TestGeneratePDF_MultiPage(t *testing.T) {
	dir := t.TempDir()
	// 21 photos → 2 pages (20 + 1).
	var srcs []string
	for i := 0; i < perPage+1; i++ {
		p := filepath.Join(dir, fmt.Sprintf("photo_%03d.jpg", i+1))
		makeTestJPEG(t, p, 500, 700)
		srcs = append(srcs, p)
	}

	out := filepath.Join(dir, "out.pdf")
	mustNoErr(t, GeneratePDF(srcs, dir, out), "GeneratePDF")

	// A 2-page PDF with 21 images should be meaningfully larger than a 1-page one.
	info, err := os.Stat(out)
	mustNoErr(t, err, "stat PDF")
	if info.Size() < 4096 {
		t.Errorf("multi-page PDF suspiciously small (%d bytes)", info.Size())
	}
}

func TestGeneratePDF_EmptyInput_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	err := GeneratePDF(nil, dir, filepath.Join(dir, "out.pdf"))
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}
