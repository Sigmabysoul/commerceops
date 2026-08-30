package pdfgenerator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

const (
	maxPages       = 500
	maxSourceBytes = 20 << 20
	maxOutputBytes = 100 << 20
)

var mediaBoxRE = regexp.MustCompile(`(?m)^MediaBox:\s+0\.00\s+0\.00\s+595\.00\s+842\.00\s*$`)

type Poppler struct{ timeout time.Duration }

func NewPoppler() *Poppler { return &Poppler{timeout: 60 * time.Second} }

func (p *Poppler) Generate(parent context.Context, pages []Page, exportInvoices bool) (Result, error) {
	if len(pages) == 0 || len(pages) > maxPages {
		return Result{}, ErrUnsupportedLayout
	}
	ctx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "commerceops-print-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir) //nolint:errcheck

	sources := make(map[string]string)
	labels, invoices := make([]string, 0, len(pages)), make([]string, 0, len(pages))
	for index, page := range pages {
		if page.SourceID == "" || page.Number < 1 || len(page.PDF) == 0 || len(page.PDF) > maxSourceBytes {
			return Result{}, ErrUnsupportedLayout
		}
		source, ok := sources[page.SourceID]
		if !ok {
			source = filepath.Join(dir, fmt.Sprintf("source-%03d.pdf", len(sources)+1))
			if err := os.WriteFile(source, page.PDF, 0o600); err != nil {
				return Result{}, err
			}
			info, commandErr := exec.CommandContext(ctx, "pdfinfo", "-box", source).CombinedOutput()
			if commandErr != nil || !mediaBoxRE.Match(info) {
				return Result{}, ErrUnsupportedLayout
			}
			sources[page.SourceID] = source
		}
		labelPrefix := filepath.Join(dir, fmt.Sprintf("label-%04d", index+1))
		if err := render(ctx, source, page.Number, 188, 26, 218, 360, labelPrefix); err != nil {
			return Result{}, err
		}
		labels = append(labels, labelPrefix+".pdf")
		if exportInvoices {
			invoicePrefix := filepath.Join(dir, fmt.Sprintf("invoice-%04d", index+1))
			if err := render(ctx, source, page.Number, 0, 386, 595, 456, invoicePrefix); err != nil {
				return Result{}, err
			}
			invoices = append(invoices, invoicePrefix+".pdf")
		}
	}
	labelOutput := filepath.Join(dir, "labels.pdf")
	if err := unite(ctx, labels, labelOutput); err != nil {
		return Result{}, err
	}
	result := Result{}
	if result.Labels, err = boundedRead(labelOutput); err != nil {
		return Result{}, err
	}
	if exportInvoices {
		invoiceOutput := filepath.Join(dir, "invoices.pdf")
		if err := unite(ctx, invoices, invoiceOutput); err != nil {
			return Result{}, err
		}
		result.Invoices, err = boundedRead(invoiceOutput)
	}
	return result, err
}

func render(ctx context.Context, source string, page, x, y, width, height int, prefix string) error {
	args := []string{"-pdf", "-nocrop", "-nocenter", "-r", "72", "-paperw", strconv.Itoa(width), "-paperh", strconv.Itoa(height), "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-x", strconv.Itoa(x), "-y", strconv.Itoa(y), "-W", strconv.Itoa(width), "-H", strconv.Itoa(height), source, prefix + ".pdf"}
	if output, err := exec.CommandContext(ctx, "pdftocairo", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("pdftocairo: %w: %.512s", err, output)
	}
	return nil
}

func unite(ctx context.Context, inputs []string, output string) error {
	args := append(append(make([]string, 0, len(inputs)+1), inputs...), output)
	if data, err := exec.CommandContext(ctx, "pdfunite", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("pdfunite: %w: %.512s", err, data)
	}
	return nil
}

func boundedRead(name string) ([]byte, error) {
	info, err := os.Stat(name)
	if err != nil || info.Size() <= 0 || info.Size() > maxOutputBytes {
		return nil, ErrUnsupportedLayout
	}
	return os.ReadFile(name)
}
