package broadcom

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultScanOutputLimit = 16 * 1024
	maximumScanOutputLimit = 64 * 1024
	maximumScanTimeout     = 30 * time.Second
	scanCommandOverhead    = 2 * time.Second
	resultsReadAttempts    = 5
	resultsReadInterval    = 250 * time.Millisecond
	resultsReadBudget      = 2 * time.Second
)

var scanInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

// PassiveScanRequest describes a manual, passive-only driver scan. The caller
// must set AllowDisruption because even passive off-channel scans may affect
// clients on the serving radio. No packet payload capture is performed.
type PassiveScanRequest struct {
	Interface       string
	Channels        []int
	PassiveDwell    time.Duration
	HomeDwell       time.Duration
	Timeout         time.Duration
	MaxOutputBytes  int
	AllowDisruption bool
}

// PassiveScanResult reports command output and the most recently available
// scanresults output. Firmware help confirms the commands, but this bridge
// does not know a reliable completion marker, so result freshness stays
// "unknown" and the data may predate the requested scan.
type PassiveScanResult struct {
	Interface        string
	Channels         []int
	StartedAt        time.Time
	CompletedAt      time.Time
	ScanOutput       string
	ResultsOutput    string
	OutputTruncated  bool
	ResultsFreshness string // "unknown"
}

// PassiveScanRunner is injectable for offline tests. A production runner
// should enforce the provided context and cap combined stdout/stderr.
type PassiveScanRunner func(context.Context, int, string, ...string) (string, bool, error)

// RunPassiveScan performs one explicit passive scan and then reads the
// driver's latest results. It never runs unless AllowDisruption is true,
// always passes -t passive, limits channels/time/output, and exposes no CLI or
// automatic scheduling by itself.
func RunPassiveScan(ctx context.Context, req PassiveScanRequest, runner PassiveScanRunner) (*PassiveScanResult, error) {
	if err := validatePassiveScanRequest(req); err != nil {
		return nil, err
	}
	if runner == nil {
		runner = runBoundedScanCommand
	}
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	started := time.Now()
	channels := make([]string, len(req.Channels))
	for i, channel := range req.Channels {
		channels[i] = strconv.Itoa(channel)
	}
	args := []string{"-i", req.Interface, "scan", "-t", "passive", "-p", strconv.FormatInt(req.PassiveDwell.Milliseconds(), 10), "-h", strconv.FormatInt(req.HomeDwell.Milliseconds(), 10), "-c", strings.Join(channels, ",")}
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = defaultScanOutputLimit
	}
	scanOutput, truncated, err := runner(ctx, limit, "wl", args...)
	if err != nil {
		return nil, fmt.Errorf("start passive scan: %w", err)
	}
	remaining := limit - len(scanOutput)
	if remaining < 0 {
		remaining = 0
	}
	resultsCtx, cancelResults := context.WithTimeout(ctx, resultsReadBudget)
	defer cancelResults()
	var results string
	var resultsTruncated bool
	var readErr error
	for attempt := 0; attempt < resultsReadAttempts; attempt++ {
		results, resultsTruncated, readErr = runner(resultsCtx, remaining, "wl", "-i", req.Interface, "scanresults")
		if readErr == nil {
			break
		}
		if resultsCtx.Err() != nil {
			return nil, fmt.Errorf("read scan results: %w", resultsCtx.Err())
		}
		if attempt+1 == resultsReadAttempts {
			break
		}
		timer := time.NewTimer(resultsReadInterval)
		select {
		case <-resultsCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, fmt.Errorf("read scan results: %w", resultsCtx.Err())
		case <-timer.C:
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("read scan results after %d attempts: %w", resultsReadAttempts, readErr)
	}
	if truncated || resultsTruncated || len(results) > remaining {
		truncated = true
		if len(results) > remaining {
			results = results[:remaining]
		}
	}
	completed := time.Now()
	return &PassiveScanResult{Interface: req.Interface, Channels: append([]int(nil), req.Channels...), StartedAt: started, CompletedAt: completed, ScanOutput: scanOutput, ResultsOutput: results, OutputTruncated: truncated, ResultsFreshness: "unknown"}, nil
}

func validatePassiveScanRequest(req PassiveScanRequest) error {
	if !req.AllowDisruption {
		return errors.New("passive scan requires explicit AllowDisruption opt-in")
	}
	if !scanInterfacePattern.MatchString(req.Interface) {
		return errors.New("invalid wireless interface")
	}
	if len(req.Channels) == 0 || len(req.Channels) > 16 {
		return errors.New("scan requires 1 to 16 explicit channels")
	}
	seen := make(map[int]bool, len(req.Channels))
	for _, channel := range req.Channels {
		if !supportedXT8ScanChannel(channel) || seen[channel] {
			return fmt.Errorf("unsupported or duplicate scan channel %d", channel)
		}
		seen[channel] = true
	}
	if req.PassiveDwell < 10*time.Millisecond || req.PassiveDwell > time.Second {
		return errors.New("passive dwell must be between 10 and 1000 ms")
	}
	if req.HomeDwell < 0 || req.HomeDwell > time.Second {
		return errors.New("home dwell must be between 0 and 1000 ms")
	}
	if req.Timeout <= 0 || req.Timeout > maximumScanTimeout {
		return fmt.Errorf("scan timeout must be between 1 ms and %s", maximumScanTimeout)
	}
	if time.Duration(len(req.Channels))*(req.PassiveDwell+req.HomeDwell)+scanCommandOverhead > req.Timeout {
		return errors.New("requested dwell budget exceeds scan timeout")
	}
	if req.MaxOutputBytes < 0 || req.MaxOutputBytes > maximumScanOutputLimit {
		return fmt.Errorf("output limit must be between 0 and %d bytes", maximumScanOutputLimit)
	}
	return nil
}

func supportedXT8ScanChannel(channel int) bool {
	if channel >= 1 && channel <= 14 {
		return true
	}
	switch channel {
	case 32, 36, 40, 44, 48, 52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144, 149, 153, 157, 161, 165, 169, 173, 177:
		return true
	default:
		return false
	}
}

func runBoundedScanCommand(ctx context.Context, limit int, name string, args ...string) (string, bool, error) {
	if limit < 0 {
		limit = 0
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 300 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	writer := &boundedScanWriter{limit: limit}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	if ctx.Err() != nil {
		return writer.buf.String(), writer.truncated, ctx.Err()
	}
	return writer.buf.String(), writer.truncated, err
}

type boundedScanWriter struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *boundedScanWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		w.truncated = w.truncated || len(data) > 0
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = w.buf.Write(data[:remaining])
		w.truncated = true
		return len(data), nil
	}
	_, _ = w.buf.Write(data)
	return len(data), nil
}
