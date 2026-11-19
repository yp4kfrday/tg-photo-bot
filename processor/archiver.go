package processor

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Pack creates a ZIP archive at destPath containing all files listed in paths.
// Entries are stored under their base names only — no directory components are
// written, making path-traversal attacks impossible.
func Pack(paths []string, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create zip %s: %w", destPath, err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	for _, src := range paths {
		if err := addFileToZip(w, src); err != nil {
			return err
		}
	}

	slog.Info("ZIP created", "path", destPath, "files", len(paths))
	return nil
}

func addFileToZip(w *zip.Writer, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("zip header for %s: %w", src, err)
	}
	// Use only base name: strips any directory prefix and prevents traversal.
	header.Name = filepath.Base(src)
	header.Method = zip.Deflate

	entry, err := w.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", header.Name, err)
	}

	if _, err = io.Copy(entry, f); err != nil {
		return fmt.Errorf("write zip entry %s: %w", header.Name, err)
	}

	slog.Debug("added to ZIP", "file", header.Name, "bytes", info.Size())
	return nil
}
