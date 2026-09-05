package printeragent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	BaseURL, Credential string
	HTTP                *http.Client
}
type claim struct {
	Job struct {
		ID     string `json:"id"`
		Copies int    `json:"copies"`
	} `json:"job"`
	LeaseToken     string `json:"lease_token"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	ArtifactSize   int64  `json:"artifact_size"`
	OSPrinterID    string `json:"os_printer_id"`
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Credential)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTP.Do(req)
}
func (c *Client) Heartbeat(ctx context.Context, p []DiscoveredPrinter) error {
	resp, err := c.do(ctx, "POST", "/api/v1/printer-agent/heartbeat", map[string]any{"printers": p})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("heartbeat status %d", resp.StatusCode)
	}
	return nil
}
func (c *Client) Claim(ctx context.Context) (claim, bool, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/printer-agent/jobs/claim", nil)
	if err != nil {
		return claim{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return claim{}, false, nil
	}
	if resp.StatusCode != 200 {
		return claim{}, false, fmt.Errorf("claim status %d", resp.StatusCode)
	}
	var envelope struct {
		Claim claim `json:"claim"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return claim{}, false, err
	}
	return envelope.Claim, true, nil
}
func (c *Client) Download(ctx context.Context, x claim) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/v1/printer-agent/jobs/"+x.Job.ID+"/artifact", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Credential)
	req.Header.Set("X-Print-Lease", x.LeaseToken)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, x.ArtifactSize+1))
	if err != nil || int64(len(data)) != x.ArtifactSize {
		return nil, errors.New("artifact size mismatch")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != x.ArtifactSHA256 {
		return nil, errors.New("artifact hash mismatch")
	}
	return data, nil
}
func (c *Client) Report(ctx context.Context, x claim, status, code, message string) error {
	resp, err := c.do(ctx, "POST", "/api/v1/printer-agent/jobs/"+x.Job.ID+"/status", map[string]any{"lease_token": x.LeaseToken, "status": status, "failure_code": code, "failure_message": message})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("report status %d", resp.StatusCode)
	}
	return nil
}

type Runner struct {
	Client  *Client
	Backend Backend
	Journal *Journal
}

func (r *Runner) Tick(ctx context.Context) error {
	printers, err := r.Backend.List(ctx)
	if err != nil {
		return err
	}
	if err = r.Client.Heartbeat(ctx, printers); err != nil {
		return err
	}
	x, ok, err := r.Client.Claim(ctx)
	if err != nil || !ok {
		return err
	}
	state := r.Journal.State(x.Job.ID)
	// A reconnect may receive a claim whose completion acknowledgement was lost.
	// The durable journal turns that into a report replay, never another print.
	if state == "submitted" || state == "completed" {
		return r.Client.Report(ctx, x, "completed", "", "")
	}
	data, err := r.Client.Download(ctx, x)
	if err != nil {
		_ = r.Client.Report(ctx, x, "failed", "download_failed", err.Error())
		return err
	}
	f, err := os.CreateTemp("", "commerceops-agent-*.pdf")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_ = f.Chmod(0o600)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = r.Client.Report(ctx, x, "printing", "", ""); err != nil {
		return err
	}
	if err = r.Journal.Record(x.Job.ID, "submitted"); err != nil {
		return err
	}
	// Recording before CUPS deliberately provides at-most-once submission. An
	// ambiguous crash becomes an operator-reviewed retry instead of duplicates.
	if err = r.Backend.SubmitPDF(ctx, x.OSPrinterID, x.Job.Copies, name); err != nil {
		_ = r.Journal.Record(x.Job.ID, "failed")
		_ = r.Client.Report(ctx, x, "failed", "cups_failed", err.Error())
		return err
	}
	if err = r.Journal.Record(x.Job.ID, "completed"); err != nil {
		return err
	}
	return r.Client.Report(ctx, x, "completed", "", "")
}
func Run(ctx context.Context, r *Runner, interval time.Duration) error {
	if interval < time.Second {
		return errors.New("interval must be at least one second")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.Tick(ctx); err != nil && ctx.Err() == nil {
			time.Sleep(time.Second)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
