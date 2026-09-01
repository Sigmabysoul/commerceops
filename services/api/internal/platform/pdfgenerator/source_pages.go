package pdfgenerator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const SourcePageGenerationVersion = "source-page-v1"

// SourcePages preserves complete source pages without marketplace-specific
// cropping or overlays. It is suitable only where the source page is already
// the printable shipping label.
type SourcePages struct{ timeout time.Duration }

func NewSourcePages() *SourcePages { return &SourcePages{timeout: 60 * time.Second} }

func (g *SourcePages) Generate(parent context.Context, pages []Page, exportInvoices bool) (Result, error) {
	if exportInvoices || len(pages) == 0 || len(pages) > maxPages {
		return Result{}, ErrUnsupportedLayout
	}
	ctx, cancel := context.WithTimeout(parent, g.timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "commerceops-source-pages-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir) //nolint:errcheck

	sources := make(map[string]string)
	labels := make([]string, 0, len(pages))
	for index, page := range pages {
		if page.SourceID == "" || page.Number < 1 || len(page.PDF) == 0 || len(page.PDF) > maxSourceBytes {
			return Result{}, ErrUnsupportedLayout
		}
		source, ok := sources[page.SourceID]
		if !ok {
			source = filepath.Join(dir, fmt.Sprintf("source-%03d.pdf", len(sources)+1))
			if err = os.WriteFile(source, page.PDF, 0o600); err != nil {
				return Result{}, err
			}
			sources[page.SourceID] = source
		}
		pattern := filepath.Join(dir, fmt.Sprintf("label-%04d-%%d.pdf", index+1))
		output := filepath.Join(dir, fmt.Sprintf("label-%04d-%d.pdf", index+1, page.Number))
		args := []string{"-f", strconv.Itoa(page.Number), "-l", strconv.Itoa(page.Number), source, pattern}
		if data, commandErr := exec.CommandContext(ctx, "pdfseparate", args...).CombinedOutput(); commandErr != nil {
			return Result{}, fmt.Errorf("pdfseparate source page: %w: %.512s", commandErr, data)
		}
		labels = append(labels, output)
	}
	output := filepath.Join(dir, "labels.pdf")
	if err = unite(ctx, labels, output); err != nil {
		return Result{}, err
	}
	data, err := boundedRead(output)
	if err != nil {
		return Result{}, err
	}
	return Result{Labels: data}, nil
}
