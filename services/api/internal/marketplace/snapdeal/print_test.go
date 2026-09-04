package snapdeal

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/commerceops/commerceops/services/api/internal/platform/pdfextractor"
	"github.com/commerceops/commerceops/services/api/internal/platform/pdfgenerator"
)

func TestPrintGeneratorEnrichesMeasuredPackslipAndExportsInvoice(t *testing.T) {
	pdf := sanitizedTwoPagePDF()
	result, err := NewPrintGenerator().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "safe", PDF: pdf, Number: 1, InvoiceNumber: 2, SKU: "9_SAFE-SKU-R1", Quantity: 2}}, true)
	if err != nil {
		t.Fatal(err)
	}
	labels, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Labels)
	if err != nil || len(labels) != 1 || !strings.Contains(labels[0].Text, "SKU: 9_SAFE-SKU-R1 | QTY: 2") {
		t.Fatalf("labels=%#v err=%v", labels, err)
	}
	invoices, err := pdfextractor.NewPoppler().Extract(context.Background(), result.Invoices)
	if err != nil || len(invoices) != 1 || !strings.Contains(invoices[0].Text, "TAX INVOICE") {
		t.Fatalf("invoices=%#v err=%v", invoices, err)
	}
	assertPreservedLabelInk(t, result.Labels)
}
func TestPrintGeneratorRejectsMissingEvidenceAndWrongGeometry(t *testing.T) {
	g := NewPrintGenerator()
	if _, err := g.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "safe", PDF: sanitizedTwoPagePDF(), Number: 1, SKU: "SAFE"}}, false); err == nil {
		t.Fatal("missing quantity accepted")
	}
	if _, err := g.Generate(context.Background(), []pdfgenerator.Page{{SourceID: "bad", PDF: []byte("%PDF-bad"), Number: 1, SKU: "SAFE", Quantity: 1}}, false); err == nil {
		t.Fatal("bad geometry accepted")
	}
}
func TestPrivateSnapdealSample(t *testing.T) {
	path := os.Getenv("SNAPDEAL_PRIVATE_SAMPLE")
	if path == "" {
		t.Skip("SNAPDEAL_PRIVATE_SAMPLE is not set")
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfextractor.NewPoppler().Extract(context.Background(), pdf)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := Parse(pages)
	if err != nil || len(docs) != 1 || docs[0].Quantity == nil || docs[0].SKU == "" || docs[0].AssociationMethod != "exact_suborder" {
		quantity, sku, associated := false, false, false
		if len(docs) == 1 {
			quantity, sku, associated = docs[0].Quantity != nil, docs[0].SKU != "", docs[0].AssociationMethod == "exact_suborder"
		}
		shippingQuantityFound, invoiceQuantityFound := false, false
		if len(pages) >= 2 {
			sq, _ := shippingQuantity(pages[0].Text, shippingOrderID(pages[0].Text))
			iq, _ := invoiceQuantity(pages[1].Text)
			shippingQuantityFound, invoiceQuantityFound = sq != nil, iq != nil
		}
		t.Fatalf("private sample structure was not recognized: documents=%d quantity=%t shipping_quantity=%t invoice_quantity=%t sku=%t associated=%t err=%v", len(docs), quantity, shippingQuantityFound, invoiceQuantityFound, sku, associated, err)
	}
	invoice := 0
	for _, s := range docs[0].Sources {
		if s.Role == "invoice" {
			invoice = s.Page
		}
	}
	result, err := NewPrintGenerator().Generate(context.Background(), []pdfgenerator.Page{{SourceID: "private", PDF: pdf, Number: docs[0].Page, InvoiceNumber: invoice, SKU: docs[0].SKU, Quantity: *docs[0].Quantity}}, true)
	if err != nil || len(result.Labels) == 0 || len(result.Invoices) == 0 {
		t.Fatalf("private print failed: %v", err)
	}
}

func sanitizedTwoPagePDF() []byte {
	content1 := []byte("0 G 2 w 20 150 257 240 re S 40 330 180 25 re f 40 280 210 12 re f")
	content2 := []byte("BT /F1 14 Tf 25 380 Td (TAX INVOICE) Tj ET")
	objects := [][]byte{[]byte("<< /Type /Catalog /Pages 2 0 R >>"), []byte("<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"), []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 297.637 419.512] /Resources << >> /Contents 5 0 R >>"), []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 297.637 419.512] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>"), pdfStream(content1), pdfStream(content2), []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	off := make([]int, len(objects)+1)
	for i, o := range objects {
		off[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(o)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i < len(off); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", off[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.Bytes()
}
func pdfStream(data []byte) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(data)) + string(data) + "\nendstream")
}

func assertPreservedLabelInk(t *testing.T, pdf []byte) {
	t.Helper()
	dir := t.TempDir()
	name := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(name, pdf, 0600); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(dir, "page")
	if output, err := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-r", "72", "-png", name, prefix).CombinedOutput(); err != nil {
		t.Fatalf("render: %s %v", output, err)
	}
	file, err := os.Open(prefix + ".png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	dark := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Min.Y+int(float64(b.Dy())*.7); y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, blue, _ := img.At(x, y).RGBA()
			if r+g+blue < 3*0x5000 {
				dark++
			}
		}
	}
	if dark < 500 {
		t.Fatalf("source label ink not preserved: %d", dark)
	}
}
