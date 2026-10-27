package capture

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const client = "02:11:22:33:44:55"

type fakeReader struct {
	packets [][]byte
	closed  bool
	readErr error
}

func (r *fakeReader) ReadPacket(ctx context.Context, dst []byte) (int, error) {
	if r.readErr != nil {
		return 0, r.readErr
	}
	if len(r.packets) > 0 {
		p := r.packets[0]
		r.packets = r.packets[1:]
		copy(dst, p)
		return len(p), nil
	}
	<-ctx.Done()
	return 0, errNoPacket
}
func (r *fakeReader) Close() error { r.closed = true; return nil }
func ethernet(src, dst string, payload byte, size int) []byte {
	p := make([]byte, size)
	s, _ := net.ParseMAC(src)
	d, _ := net.ParseMAC(dst)
	copy(p[:6], d)
	copy(p[6:12], s)
	p[12], p[13] = 8, 0
	for i := 14; i < len(p); i++ {
		p[i] = payload
	}
	return p
}
func request() Request {
	return Request{Interface: "br0", ClientMAC: client, Duration: 30 * time.Millisecond, PacketLimit: 100, SnapLen: 128, Allow: true}
}

func TestRunFiltersExactClientAndWritesPrivateBoundedPCAP(t *testing.T) {
	dir := t.TempDir()
	other := "02:aa:bb:cc:dd:ee"
	reader := &fakeReader{packets: [][]byte{
		ethernet(other, other, 0xa1, 80),
		ethernet(other, client, 0xb2, 200),
		ethernet(client, other, 0xc3, 60),
	}}
	opened := ""
	path, err := run(context.Background(), request(), func(name string) (PacketReader, error) { opened = name; return reader, nil }, dir)
	if err != nil {
		t.Fatal(err)
	}
	if opened != "br0" || !reader.closed {
		t.Fatalf("socket lifecycle/interface: opened=%q closed=%v", opened, reader.closed)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 || st.Size() > MaxPCAPSize {
		t.Fatalf("artifact mode=%#o size=%d", st.Mode().Perm(), st.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 24 || binary.LittleEndian.Uint32(data[:4]) != 0xa1b2c3d4 || binary.LittleEndian.Uint32(data[20:24]) != 1 {
		t.Fatalf("invalid Ethernet pcap header: %x", data[:min(24, len(data))])
	}
	off, count := 24, 0
	for off < len(data) {
		if off+16 > len(data) {
			t.Fatal("truncated pcap record header")
		}
		incl := int(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		orig := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		off += 16
		if incl < 14 || incl > 128 || orig < incl || off+incl > len(data) {
			t.Fatalf("bad record lengths incl=%d orig=%d", incl, orig)
		}
		frame := data[off : off+incl]
		if !matchesMAC(frame, mustMAC(t, client)) {
			t.Fatal("pcap contains a nonmatching frame")
		}
		if incl != 128 && incl != 60 {
			t.Fatalf("unexpected capture length %d", incl)
		}
		if incl == 128 && orig != 200 {
			t.Fatalf("snap-truncated original length=%d, want 200", orig)
		}
		off += incl
		count++
	}
	if count != 2 {
		t.Fatalf("captured %d packets, want 2", count)
	}
}

func TestRunStopsAtPacketAndTotalByteCaps(t *testing.T) {
	dir := t.TempDir()
	frames := make([][]byte, MaxPackets+1)
	for i := range frames {
		frames[i] = ethernet(client, "02:aa:bb:cc:dd:ee", byte(i), MaxSnapLen)
	}
	reader := &fakeReader{packets: frames}
	r := request()
	path, err := run(context.Background(), r, func(string) (PacketReader, error) { return reader, nil }, dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 24+MaxPackets*(16+MaxSnapLen) || info.Size() > MaxPCAPSize {
		t.Fatalf("max capture size=%d, want <=%d and exactly bounded packet output", info.Size(), MaxPCAPSize)
	}
	if len(reader.packets) != 1 {
		t.Fatalf("reader consumed %d packets, want exactly %d", MaxPackets+1-len(reader.packets), MaxPackets)
	}
}

func TestValidationPrecedesSocketAndRejectsUnsafeBounds(t *testing.T) {
	base := request()
	called := false
	for _, mutate := range []func(*Request){
		func(r *Request) { r.Allow = false }, func(r *Request) { r.Interface = "eth0" }, func(r *Request) { r.ClientMAC = "ff:ff:ff:ff:ff:ff" },
		func(r *Request) { r.Duration = MaxDuration + time.Nanosecond }, func(r *Request) { r.PacketLimit = MaxPackets + 1 },
		func(r *Request) { r.SnapLen = MaxSnapLen + 1 }, func(r *Request) { r.SnapLen = 13 },
	} {
		r := base
		mutate(&r)
		if _, err := run(context.Background(), r, func(string) (PacketReader, error) { called = true; return &fakeReader{}, nil }, t.TempDir()); err == nil {
			t.Fatalf("unsafe request accepted: %+v", r)
		}
	}
	if called {
		t.Fatal("invalid request opened packet socket")
	}
}

func TestRunTimeoutCreatesEmptyPCAPAndReaderErrorsRemoveArtifact(t *testing.T) {
	dir := t.TempDir()
	reader := &fakeReader{}
	r := request()
	r.Duration = 15 * time.Millisecond
	path, err := run(context.Background(), r, func(string) (PacketReader, error) { return reader, nil }, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 24 {
		t.Fatalf("timeout pcap size=%d, want header-only 24", len(data))
	}
	badDir := filepath.Join(dir, "error")
	if err := os.Mkdir(badDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bad := &fakeReader{readErr: errors.New("receive failure")}
	if _, err := run(context.Background(), request(), func(string) (PacketReader, error) { return bad, nil }, badDir); err == nil {
		t.Fatal("expected read error")
	}
	entries, err := os.ReadDir(badDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed capture left artifact: %s", strings.Join([]string{entries[0].Name()}, ","))
	}
	if !bad.closed {
		t.Fatal("reader not closed after error")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := &fakeReader{}
	if _, err := run(cancelCtx, request(), func(string) (PacketReader, error) { return canceled, nil }, filepath.Join(dir, "error")); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation error=%v", err)
	}
	if !canceled.closed {
		t.Fatal("reader not closed after cancellation")
	}
}

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	m, e := net.ParseMAC(s)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
