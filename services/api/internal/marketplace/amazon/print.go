// This file builds marketplace-specific printable output while keeping layout knowledge inside the adapter in the Amazon marketplace adapter.
package amazon

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
)

const PrintGenerationVersion = "amazon-a4-enriched-v1"

var amazonA4MediaBoxRE = regexp.MustCompile(`(?m)^MediaBox:\s+0\.00\s+0\.00\s+595\.00\s+842\.00\s*$`)

type PrintGenerator struct{ timeout time.Duration }

func NewPrintGenerator() *PrintGenerator { return &PrintGenerator{timeout: 2 * time.Minute} }

// Generate preserves the complete source label, scaling it uniformly into a
// validated A4 page and reserving a top banner for the explicit SKU/quantity.
func (g *PrintGenerator) Generate(parent context.Context, pages []pdfgenerator.Page, exportInvoices bool) (pdfgenerator.Result, error) {
	if len(pages) == 0 || len(pages) > 500 {
		return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
	}
	ctx, cancel := context.WithTimeout(parent, g.timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "commerceops-amazon-print-*")
	if err != nil {
		return pdfgenerator.Result{}, err
	}
	defer os.RemoveAll(dir)

	sources := map[string]string{}
	labels, invoices := []string{}, []string{}
	for index, page := range pages {
		if page.SourceID == "" || page.Number < 1 || page.SKU == "" || page.Quantity < 1 || len(page.PDF) == 0 || len(page.PDF) > 20<<20 {
			return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
		}
		source, ok := sources[page.SourceID]
		if !ok {
			source = filepath.Join(dir, fmt.Sprintf("source-%03d.pdf", len(sources)+1))
			if err = os.WriteFile(source, page.PDF, 0o600); err != nil {
				return pdfgenerator.Result{}, err
			}
			info, commandErr := exec.CommandContext(ctx, "pdfinfo", "-box", source).CombinedOutput()
			if commandErr != nil || !amazonA4MediaBoxRE.Match(info) {
				return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
			}
			sources[page.SourceID] = source
		}
		label, generateErr := g.enrichedLabel(ctx, dir, source, page, index+1)
		if generateErr != nil {
			return pdfgenerator.Result{}, generateErr
		}
		labels = append(labels, label)
		if exportInvoices {
			if page.InvoiceNumber < 1 {
				return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
			}
			prefix := filepath.Join(dir, fmt.Sprintf("invoice-%04d", index+1))
			args := []string{"-pdf", "-nocrop", "-nocenter", "-r", "72", "-paperw", "595", "-paperh", "842", "-f", strconv.Itoa(page.InvoiceNumber), "-l", strconv.Itoa(page.InvoiceNumber), "-x", "0", "-y", "0", "-W", "595", "-H", "842", source, prefix + ".pdf"}
			if output, commandErr := exec.CommandContext(ctx, "pdftocairo", args...).CombinedOutput(); commandErr != nil {
				return pdfgenerator.Result{}, fmt.Errorf("render Amazon invoice: %w: %.256s", commandErr, output)
			}
			invoices = append(invoices, prefix+".pdf")
		}
	}
	labelOutput := filepath.Join(dir, "labels.pdf")
	if err = uniteAmazon(ctx, labels, labelOutput); err != nil {
		return pdfgenerator.Result{}, err
	}
	result := pdfgenerator.Result{}
	if result.Labels, err = boundedAmazonRead(labelOutput); err != nil {
		return pdfgenerator.Result{}, err
	}
	if exportInvoices {
		invoiceOutput := filepath.Join(dir, "invoices.pdf")
		if err = uniteAmazon(ctx, invoices, invoiceOutput); err != nil {
			return pdfgenerator.Result{}, err
		}
		result.Invoices, err = boundedAmazonRead(invoiceOutput)
	}
	return result, err
}

func (g *PrintGenerator) enrichedLabel(ctx context.Context, dir, source string, page pdfgenerator.Page, position int) (string, error) {
	prefix := filepath.Join(dir, fmt.Sprintf("render-%04d", position))
	args := []string{"-f", strconv.Itoa(page.Number), "-l", strconv.Itoa(page.Number), "-singlefile", "-r", "200", "-png", source, prefix}
	if output, err := exec.CommandContext(ctx, "pdftoppm", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("render Amazon label: %w: %.256s", err, output)
	}
	file, err := os.Open(prefix + ".png")
	if err != nil {
		return "", err
	}
	decoded, decodeErr := png.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return "", decodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	bounds := decoded.Bounds()
	if bounds.Dx() < 1000 || bounds.Dy() < 1400 || bounds.Dx() > 2500 || bounds.Dy() > 3500 {
		return "", pdfgenerator.ErrUnsupportedLayout
	}
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	raw := make([]byte, 0, bounds.Dx()*bounds.Dy()*3)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			offset := rgba.PixOffset(x, y)
			raw = append(raw, rgba.Pix[offset], rgba.Pix[offset+1], rgba.Pix[offset+2])
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err = zw.Write(raw); err != nil {
		return "", err
	}
	if err = zw.Close(); err != nil {
		return "", err
	}
	banner := "SKU: " + page.SKU + " | QTY: " + strconv.Itoa(page.Quantity)
	output := filepath.Join(dir, fmt.Sprintf("label-%04d.pdf", position))
	if err = writeEnrichedPDF(output, bounds.Dx(), bounds.Dy(), compressed.Bytes(), banner); err != nil {
		return "", err
	}
	return output, nil
}

func writeEnrichedPDF(name string, width, height int, imageData []byte, banner string) error {
	fontSize := 28.0
	if estimated := float64(len(banner)) * fontSize * 0.62; estimated > 555 {
		fontSize = 555 / (float64(len(banner)) * 0.62)
	}
	if fontSize < 14 {
		return pdfgenerator.ErrUnsupportedLayout
	}
	textWidth := float64(len(banner)) * fontSize * 0.62
	x := (595 - textWidth) / 2
	if x < 20 {
		x = 20
	}
	content := fmt.Sprintf("q 544.12 0 0 770 25.44 0 cm /Im0 Do Q\n0.94 g 0 770 595 72 re f\n0 G 1.5 w 0 770 595 72 re S\nBT /F1 %.2f Tf %.2f 795 Td (%s) Tj ET\n", fontSize, x, escapePDFText(banner))
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /XObject << /Im0 5 0 R >> /Font << /F1 6 0 R >> >> /Contents 4 0 R >>"),
		streamObject([]byte(content), ""),
		streamObject(imageData, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", width, height)),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>"),
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", index+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(name, pdf.Bytes(), 0o600)
}

func streamObject(data []byte, dictionary string) []byte {
	return []byte(fmt.Sprintf("<< %s /Length %d >>\nstream\n", dictionary, len(data)) + string(data) + "\nendstream")
}

func escapePDFText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "(", `\(`)
	return strings.ReplaceAll(value, ")", `\)`)
}

func uniteAmazon(ctx context.Context, inputs []string, output string) error {
	args := append(append([]string{}, inputs...), output)
	if data, err := exec.CommandContext(ctx, "pdfunite", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("pdfunite Amazon output: %w: %.256s", err, data)
	}
	return nil
}

func boundedAmazonRead(name string) ([]byte, error) {
	info, err := os.Stat(name)
	if err != nil || info.Size() <= 0 || info.Size() > 100<<20 {
		return nil, pdfgenerator.ErrUnsupportedLayout
	}
	return os.ReadFile(name)
}
