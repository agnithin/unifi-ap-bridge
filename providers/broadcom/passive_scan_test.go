package broadcom

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validPassiveScanRequest() PassiveScanRequest {
	return PassiveScanRequest{Interface: "eth4", Channels: []int{1, 6, 36}, PassiveDwell: 100 * time.Millisecond, HomeDwell: 50 * time.Millisecond, Timeout: time.Second, AllowDisruption: true}
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
