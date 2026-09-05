package printeragent

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var printerIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`)

type CUPS struct{}

func NewCUPS() *CUPS { return &CUPS{} }
func (c *CUPS) List(ctx context.Context) ([]DiscoveredPrinter, error) {
	out, err := exec.CommandContext(ctx, "lpstat", "-p").Output()
	if err != nil {
		return nil, fmt.Errorf("list CUPS printers: %w", err)
	}
	items := []DiscoveredPrinter{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "printer" && printerIDRE.MatchString(fields[1]) {
			items = append(items, DiscoveredPrinter{OSPrinterID: fields[1], SuggestedName: fields[1], Capabilities: map[string]any{"backend": "cups", "format": "application/pdf"}})
		}
	}
	return items, nil
}
func (c *CUPS) SubmitPDF(ctx context.Context, printer string, copies int, filename string) error {
	if !printerIDRE.MatchString(printer) || copies < 1 || copies > 100 || filename == "" {
		return errors.New("invalid CUPS submission")
	}
	out, err := exec.CommandContext(ctx, "lp", "-d", printer, "-n", fmt.Sprintf("%d", copies), "--", filename).CombinedOutput()
	if err != nil {
		return fmt.Errorf("CUPS submission failed: %w: %.256s", err, out)
	}
	return nil
}
