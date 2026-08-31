package pdfextractor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var pagesRE = regexp.MustCompile(`(?m)^Pages:\s+([0-9]+)\s*$`)

const maxOCRImageBytes = 50 << 20

// Poppler uses the mature pdfinfo/pdftotext boundary for untrusted PDF
// decoding. It returns page-delimited text only; business parsing stays in Go.
type Poppler struct {
	timeout       time.Duration
	ocrEmptyPages bool
}

func NewPoppler() *Poppler { return &Poppler{timeout: 30 * time.Second} }
func NewPopplerWithOCR() *Poppler {
	return &Poppler{timeout: 2 * time.Minute, ocrEmptyPages: true}
}
func (p *Poppler) Extract(parent context.Context, pdf []byte) ([]Page, error) {
	ctx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()
	file, err := os.CreateTemp("", "commerceops-*.pdf")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if _, err = file.Write(pdf); err != nil {
		file.Close()
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	info, err := exec.CommandContext(ctx, "pdfinfo", name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pdfinfo: %w", err)
	}
	match := pagesRE.FindSubmatch(info)
	if len(match) != 2 {
		return nil, errors.New("PDF page count unavailable")
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil || count < 1 || count > MaxPages {
		return nil, fmt.Errorf("PDF page count outside 1-%d", MaxPages)
	}
	cmd := exec.CommandContext(ctx, "pdftotext", "-f", "1", "-l", strconv.Itoa(count), "-layout", "-enc", "UTF-8", name, "-")
	var output limitedBuffer
	output.limit = MaxDocumentTextBytes + 1
	cmd.Stdout = &output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext: %w", err)
	}
	if output.exceeded {
		return nil, errors.New("extracted document text exceeds limit")
	}
	raw := strings.Split(strings.ReplaceAll(output.String(), "\r\n", "\n"), "\f")
	if len(raw) > 0 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1]
	}
	if len(raw) != count {
		return nil, fmt.Errorf("expected %d extracted pages, got %d", count, len(raw))
	}
	pages := make([]Page, 0, count)
	for index, text := range raw {
		if len(text) > MaxPageTextBytes {
			return nil, fmt.Errorf("page %d text exceeds limit", index+1)
		}
		method := "text"
		if strings.TrimSpace(text) == "" && p.ocrEmptyPages {
			text, err = p.ocrPage(ctx, name, index+1)
			if err != nil {
				return nil, err
			}
			method = "ocr"
		}
		pages = append(pages, Page{Number: index + 1, Text: text, ExtractionMethod: method})
	}
	return pages, nil
}

func (p *Poppler) ocrPage(ctx context.Context, pdf string, page int) (string, error) {
	dir, err := os.MkdirTemp("", "commerceops-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	if output, commandErr := exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", "-r", "300", "-png", pdf, prefix).CombinedOutput(); commandErr != nil {
		return "", fmt.Errorf("render page %d for OCR: %w", page, commandErr)
	} else if len(output) > MaxPageTextBytes {
		return "", fmt.Errorf("page %d renderer output exceeds limit", page)
	}
	info, err := os.Stat(prefix + ".png")
	if err != nil {
		return "", fmt.Errorf("inspect rendered page %d: %w", page, err)
	}
	if info.Size() > maxOCRImageBytes {
		return "", fmt.Errorf("page %d OCR image exceeds limit", page)
	}
	cmd := exec.CommandContext(ctx, "tesseract", prefix+".png", "stdout", "--psm", "6")
	var output limitedBuffer
	output.limit = MaxPageTextBytes + 1
	cmd.Stdout = &output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("OCR page %d: %w", page, err)
	}
	if output.exceeded {
		return "", fmt.Errorf("page %d OCR text exceeds limit", page)
	}
	return strings.ReplaceAll(output.String(), "\r\n", "\n"), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.exceeded = true
		return original, nil
	}
	if len(value) > remaining {
		b.exceeded = true
		value = value[:remaining]
	}
	_, _ = b.Buffer.Write(value)
	return original, nil
}
