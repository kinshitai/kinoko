//go:build embedding

package embedding

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Model and runtime download URLs.
const (
	modelONNXURL     = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/onnx/model.onnx"
	tokenizerURL     = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main/tokenizer.json"
	ortVersion       = "1.22.0"
	ortDownloadURL   = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-linux-x64-1.22.0.tgz"

	// Minimum expected file sizes for basic verification.
	minModelSize     = 30_000_000 // ~33MB
	minTokenizerSize = 500_000    // ~695KB
	minORTLibSize    = 10_000_000 // ~25MB
)

// EnsureModel checks that modelDir contains all required files.
// Missing files are downloaded. Returns an error if any download fails.
func EnsureModel(modelDir string) error {
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}

	files := []struct {
		path    string
		url     string
		minSize int64
		desc    string
	}{
		{filepath.Join(modelDir, "model.onnx"), modelONNXURL, minModelSize, "ONNX model"},
		{filepath.Join(modelDir, "tokenizer.json"), tokenizerURL, minTokenizerSize, "tokenizer"},
		{filepath.Join(modelDir, "libonnxruntime.so"), "", minORTLibSize, "ONNX Runtime"},
	}

	for _, f := range files {
		info, err := os.Stat(f.path)
		if err == nil && info.Size() >= f.minSize {
			continue
		}

		if f.url == "" {
			if err := downloadORT(modelDir); err != nil {
				return err
			}
			continue
		}

		fmt.Printf("Downloading %s...\n", f.desc)
		if err := downloadFile(f.path, f.url, f.minSize); err != nil {
			return fmt.Errorf("download %s: %w", f.desc, err)
		}
	}

	return nil
}

func downloadFile(dst, url string, minSize int64) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	var w io.Writer = f
	if resp.ContentLength > 0 {
		w = &progressWriter{w: f, total: resp.ContentLength}
	}

	n, err := io.Copy(w, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if n < minSize {
		os.Remove(tmp)
		return fmt.Errorf("file too small: got %d bytes, expected at least %d", n, minSize)
	}

	return os.Rename(tmp, dst)
}

func downloadORT(modelDir string) error {
	fmt.Println("Downloading ONNX Runtime...")

	resp, err := http.Get(ortDownloadURL)
	if err != nil {
		return fmt.Errorf("download ORT: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading ORT", resp.StatusCode)
	}

	// Extract libonnxruntime.so directly from tarball stream.
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tarball: %w", err)
		}

		// Match lib/libonnxruntime.so.* (the actual shared lib, not symlinks).
		base := filepath.Base(hdr.Name)
		if strings.HasPrefix(base, "libonnxruntime.so") && hdr.Typeflag == tar.TypeReg && hdr.Size > minORTLibSize {
			dst := filepath.Join(modelDir, "libonnxruntime.so")
			tmp := dst + ".tmp"
			f, err := os.Create(tmp)
			if err != nil {
				return err
			}
			pw := &progressWriter{w: f, total: hdr.Size}
			if _, err := io.Copy(pw, tr); err != nil {
				f.Close()
				os.Remove(tmp)
				return err
			}
			f.Close()
			if err := os.Rename(tmp, dst); err != nil {
				return err
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("libonnxruntime.so not found in ORT tarball")
	}

	return nil
}

// progressWriter wraps an io.Writer and prints download progress.
type progressWriter struct {
	w       io.Writer
	total   int64
	written int64
	lastPct int
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := int(pw.written * 100 / pw.total)
		if pct != pw.lastPct && pct%10 == 0 {
			fmt.Printf("  %d%%\n", pct)
			pw.lastPct = pct
		}
	}
	return n, err
}
