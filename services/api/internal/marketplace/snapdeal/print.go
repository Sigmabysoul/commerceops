// This file builds marketplace-specific printable output while keeping layout knowledge inside the adapter in the Snapdeal marketplace adapter.
package snapdeal

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

const PrintGenerationVersion = "snapdeal-packslip-enriched-v1"

var sampleMediaBoxRE = regexp.MustCompile(`(?m)^MediaBox:\s+0\.00\s+0\.00\s+297\.(?:63|64)\s+419\.51\s*$`)

type PrintGenerator struct{ timeout time.Duration }

func NewPrintGenerator() *PrintGenerator { return &PrintGenerator{2 * time.Minute} }

// Generate preserves the measured 297.637x419.512 pt packslip page. The
// enrichment occupies only the verified blank band below the label body and
// above the original footer page number.
func (g *PrintGenerator) Generate(parent context.Context, pages []pdfgenerator.Page, exportInvoices bool) (pdfgenerator.Result, error) {
	if len(pages) == 0 || len(pages) > 500 {
		return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
	}
	ctx, cancel := context.WithTimeout(parent, g.timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "commerceops-snapdeal-print-*")
	if err != nil {
		return pdfgenerator.Result{}, err
	}
	defer os.RemoveAll(dir) //nolint:errcheck
	sources := map[string]string{}
	labels, invoices := []string{}, []string{}
	for i, page := range pages {
		if page.SourceID == "" || page.Number < 1 || page.SKU == "" || page.Quantity < 1 || len(page.PDF) == 0 || len(page.PDF) > 20<<20 {
			return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
		}
		source, ok := sources[page.SourceID]
		if !ok {
			source = filepath.Join(dir, fmt.Sprintf("source-%03d.pdf", len(sources)+1))
			if err = os.WriteFile(source, page.PDF, 0600); err != nil {
				return pdfgenerator.Result{}, err
			}
			info, e := exec.CommandContext(ctx, "pdfinfo", "-box", source).CombinedOutput()
			if e != nil || !sampleMediaBoxRE.Match(info) {
				return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
			}
			sources[page.SourceID] = source
		}
		label, e := g.enrich(ctx, dir, source, page, i+1)
		if e != nil {
			return pdfgenerator.Result{}, e
		}
		labels = append(labels, label)
		if exportInvoices {
			if page.InvoiceNumber < 1 {
				return pdfgenerator.Result{}, pdfgenerator.ErrUnsupportedLayout
			}
			pattern := filepath.Join(dir, fmt.Sprintf("invoice-%04d-%%d.pdf", i+1))
			if output, e := exec.CommandContext(ctx, "pdfseparate", "-f", strconv.Itoa(page.InvoiceNumber), "-l", strconv.Itoa(page.InvoiceNumber), source, pattern).CombinedOutput(); e != nil {
				return pdfgenerator.Result{}, fmt.Errorf("extract Snapdeal invoice: %w: %.256s", e, output)
			}
			invoices = append(invoices, fmt.Sprintf(pattern, page.InvoiceNumber))
		}
	}
	result := pdfgenerator.Result{}
	labelOut := filepath.Join(dir, "labels.pdf")
	if err = unite(ctx, labels, labelOut); err != nil {
		return result, err
	}
	if result.Labels, err = boundedRead(labelOut); err != nil {
		return result, err
	}
	if exportInvoices {
		invoiceOut := filepath.Join(dir, "invoices.pdf")
		if err = unite(ctx, invoices, invoiceOut); err != nil {
			return result, err
		}
		result.Invoices, err = boundedRead(invoiceOut)
	}
	return result, err
}
func (g *PrintGenerator) enrich(ctx context.Context, dir, source string, page pdfgenerator.Page, pos int) (string, error) {
	prefix := filepath.Join(dir, fmt.Sprintf("render-%04d", pos))
	if output, err := exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(page.Number), "-l", strconv.Itoa(page.Number), "-singlefile", "-r", "200", "-png", source, prefix).CombinedOutput(); err != nil {
		return "", fmt.Errorf("render Snapdeal label: %w: %.256s", err, output)
	}
	f, err := os.Open(prefix + ".png")
	if err != nil {
		return "", err
	}
	decoded, de := png.Decode(f)
	ce := f.Close()
	if de != nil {
		return "", de
	}
	if ce != nil {
		return "", ce
	}
	b := decoded.Bounds()
	if b.Dx() < 800 || b.Dx() > 850 || b.Dy() < 1150 || b.Dy() > 1180 {
		return "", pdfgenerator.ErrUnsupportedLayout
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), decoded, b.Min, draw.Src)
	raw := make([]byte, 0, b.Dx()*b.Dy()*3)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			o := rgba.PixOffset(x, y)
			raw = append(raw, rgba.Pix[o], rgba.Pix[o+1], rgba.Pix[o+2])
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
	out := filepath.Join(dir, fmt.Sprintf("label-%04d.pdf", pos))
	if err = writePDF(out, b.Dx(), b.Dy(), compressed.Bytes(), "SKU: "+page.SKU+" | QTY: "+strconv.Itoa(page.Quantity)); err != nil {
		return "", err
	}
	return out, nil
}
func writePDF(name string, w, h int, img []byte, banner string) error {
	font := 16.0
	if est := float64(len(banner)) * font * .62; est > 263 {
		font = 263 / (float64(len(banner)) * .62)
	}
	if font < 8 {
		return pdfgenerator.ErrUnsupportedLayout
	}
	tw := float64(len(banner)) * font * .62
	x := (297.637 - tw) / 2
	if x < 12 {
		x = 12
	}
	content := fmt.Sprintf("q 297.637 0 0 419.512 0 0 cm /Im0 Do Q\n1 g 12 55 273.637 42 re f\n0 G 1 w 12 55 273.637 42 re S\nBT /F1 %.2f Tf %.2f 70 Td (%s) Tj ET\n", font, x, escape(banner))
	objects := [][]byte{[]byte("<< /Type /Catalog /Pages 2 0 R >>"), []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"), []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 297.637 419.512] /Resources << /XObject << /Im0 5 0 R >> /Font << /F1 6 0 R >> >> /Contents 4 0 R >>"), stream([]byte(content), ""), stream(img, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", w, h)), []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, o := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(o)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(name, pdf.Bytes(), 0600)
}
func stream(data []byte, dict string) []byte {
	return []byte(fmt.Sprintf("<< %s /Length %d >>\nstream\n", dict, len(data)) + string(data) + "\nendstream")
}
func escape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "(", `\(`)
	return strings.ReplaceAll(v, ")", `\)`)
}
func unite(ctx context.Context, in []string, out string) error {
	args := append(append([]string{}, in...), out)
	if data, err := exec.CommandContext(ctx, "pdfunite", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("unite Snapdeal output: %w: %.256s", err, data)
	}
	return nil
}
func boundedRead(name string) ([]byte, error) {
	i, e := os.Stat(name)
	if e != nil || i.Size() <= 0 || i.Size() > 100<<20 {
		return nil, pdfgenerator.ErrUnsupportedLayout
	}
	return os.ReadFile(name)
}
