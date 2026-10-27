package broadcom

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func validPassiveScanRequest() PassiveScanRequest {
	return PassiveScanRequest{Interface: "eth4", Channels: []int{1, 6, 36}, PassiveDwell: 100 * time.Millisecond, HomeDwell: 50 * time.Millisecond, Timeout: 5 * time.Second, AllowDisruption: true}
}

func TestRunPassiveScanRequiresOptInAndBoundsRequest(t *testing.T) {
	called := false
	runner := func(context.Context, int, string, ...string) (string, bool, error) {
		called = true
		return "", false, nil
	}
	req := validPassiveScanRequest()
	req.AllowDisruption = false
	if _, err := RunPassiveScan(context.Background(), req, runner); err == nil || called {
		t.Fatalf("missing opt-in must reject before runner, called=%v err=%v", called, err)
	}
	for _, mutate := range []func(*PassiveScanRequest){
		func(r *PassiveScanRequest) { r.Interface = "eth4; reboot" },
		func(r *PassiveScanRequest) { r.Channels = nil },
		func(r *PassiveScanRequest) { r.Channels = []int{36, 36} },
		func(r *PassiveScanRequest) { r.Channels = []int{6, 200} },
		func(r *PassiveScanRequest) { r.PassiveDwell = 2 * time.Second },
		func(r *PassiveScanRequest) { r.Timeout = maximumScanTimeout + time.Second },
		func(r *PassiveScanRequest) {
			r.Channels = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 32, 36}
			r.PassiveDwell = time.Second
			r.HomeDwell = time.Second
			r.Timeout = maximumScanTimeout
		},
		func(r *PassiveScanRequest) { r.MaxOutputBytes = maximumScanOutputLimit + 1 },
	} {
		bad := validPassiveScanRequest()
		mutate(&bad)
		if _, err := RunPassiveScan(context.Background(), bad, runner); err == nil {
			t.Fatalf("invalid request was accepted: %+v", bad)
		}
	}
	if called {
		t.Fatal("invalid requests must not invoke runner")
	}
}

func TestRunBoundedScanCommandKillsOwnedProcessGroupOnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	output, _, err := runBoundedScanCommand(ctx, 100, "sh", "-c", "sleep 10 & echo $!; wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, output=%q err=%v", output, err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || childPID <= 0 {
		t.Fatalf("test child pid missing from %q: %v", output, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err = syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	// `ps` is only used to produce useful failure detail; no external system is touched.
	ps, _ := exec.Command("ps", "-o", "pid=,stat=,command=", "-p", strconv.Itoa(childPID)).Output()
	t.Fatalf("timed-out scan left child process alive: %s", ps)
}

func TestRunPassiveScanUsesExplicitPassiveChannelsAndUnknownFreshness(t *testing.T) {
	var calls [][]string
	runner := func(_ context.Context, _ int, name string, args ...string) (string, bool, error) {
		if name != "wl" {
			t.Fatalf("unexpected executable %q", name)
		}
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return "scan accepted", false, nil
		}
		return "chanspec bssid SSID\n0x1006 00:11:22:33:44:55 neighbor", false, nil
	}
	result, err := RunPassiveScan(context.Background(), validPassiveScanRequest(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-i", "eth4", "scan", "-t", "passive", "-p", "100", "-h", "50", "-c", "1,6,36"}
	if len(calls) != 2 || !reflect.DeepEqual(calls[0], want) || !reflect.DeepEqual(calls[1], []string{"-i", "eth4", "scanresults"}) {
		t.Fatalf("unexpected command sequence: %#v", calls)
	}
	if result.ResultsFreshness != "unknown" || result.OutputTruncated || !strings.Contains(result.ResultsOutput, "neighbor") {
		t.Fatalf("unexpected result contract: %+v", result)
	}
	if !result.CompletedAt.After(result.StartedAt) {
		t.Fatalf("missing timestamps: %+v", result)
	}
}

func TestRunPassiveScanRetriesOnlyReadOnlyResults(t *testing.T) {
	var starts, reads int
	runner := func(_ context.Context, _ int, _ string, args ...string) (string, bool, error) {
		if len(args) >= 3 && args[2] == "scan" {
			starts++
			return "accepted", false, nil
		}
		reads++
		if reads == 1 {
			return "", false, errors.New("results not ready")
		}
		return "latest results", false, nil
	}
	result, err := RunPassiveScan(context.Background(), validPassiveScanRequest(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 || reads != 2 || result.ScanOutput != "accepted" || result.ResultsOutput != "latest results" || result.ResultsFreshness != "unknown" {
		t.Fatalf("unexpected retry behavior: starts=%d reads=%d result=%+v", starts, reads, result)
	}
}

func TestRunPassiveScanBoundsReadRetriesAndCancellation(t *testing.T) {
	var starts, reads int
	runner := func(_ context.Context, _ int, _ string, args ...string) (string, bool, error) {
		if len(args) >= 3 && args[2] == "scan" {
			starts++
			return "accepted", false, nil
		}
		reads++
		return "", false, errors.New("not ready")
	}
	_, err := RunPassiveScan(context.Background(), validPassiveScanRequest(), runner)
	if err == nil || starts != 1 || reads != resultsReadAttempts || !strings.Contains(err.Error(), "after 5 attempts") {
		t.Fatalf("read retries were not capped: starts=%d reads=%d err=%v", starts, reads, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	starts, reads = 0, 0
	cancelRunner := func(_ context.Context, _ int, _ string, args ...string) (string, bool, error) {
		if len(args) >= 3 && args[2] == "scan" {
			starts++
			return "accepted", false, nil
		}
		reads++
		cancel()
		return "", false, errors.New("not ready")
	}
	_, err = RunPassiveScan(ctx, validPassiveScanRequest(), cancelRunner)
	if !errors.Is(err, context.Canceled) || starts != 1 || reads != 1 {
		t.Fatalf("cancelled result wait was not prompt: starts=%d reads=%d err=%v", starts, reads, err)
	}
}

func TestRunPassiveScanCapsCombinedOutputAndHonorsRunnerErrors(t *testing.T) {
	req := validPassiveScanRequest()
	req.MaxOutputBytes = 12
	call := 0
	runner := func(_ context.Context, limit int, _ string, _ ...string) (string, bool, error) {
		call++
		if call == 1 && limit != 12 {
			t.Fatalf("first output limit=%d", limit)
		}
		if call == 2 && limit != 7 {
			t.Fatalf("remaining output limit=%d", limit)
		}
		if call == 1 {
			return "12345", false, nil
		}
		return "abcdefgh", true, nil
	}
	result, err := RunPassiveScan(context.Background(), req, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputTruncated || len(result.ScanOutput)+len(result.ResultsOutput) != 12 || result.ResultsOutput != "abcdefg" {
		t.Fatalf("output was not bounded: %+v", result)
	}
	_, err = RunPassiveScan(context.Background(), validPassiveScanRequest(), func(_ context.Context, _ int, _ string, _ ...string) (string, bool, error) {
		return "", false, errors.New("fixture failure")
	})
	if err == nil {
		t.Fatal("runner error should be returned")
	}
}
