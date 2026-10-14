package kernel

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKernel prints its version, echoes the config path and then sleeps, or
// exits immediately when the config contains "crash".
const fakeKernel = `#!/bin/sh
if [ "$1" = "version" ]; then echo "Xray 9.9.9 (fake)"; exit 0; fi
echo "starting with $3"
if grep -q crash "$3"; then echo "config error"; exit 1; fi
exec sleep 300
`

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(0o755)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type nftRecorder struct {
	mu      sync.Mutex
	scripts []string
	listOut string
}

func stubNft(t *testing.T) *nftRecorder {
	t.Helper()
	rec := &nftRecorder{}
	oldRunner, oldAvail := nftRunner, nftAvailable
	nftRunner = func(_ context.Context, stdin string, args ...string) ([]byte, error) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		joined := strings.Join(args, " ")
		switch {
		case joined == "-f -":
			rec.scripts = append(rec.scripts, stdin)
			return nil, nil
		case strings.HasPrefix(joined, "list table"):
			if len(rec.scripts) == 0 {
				return nil, errors.New("no table")
			}
			return nil, nil
		case strings.HasPrefix(joined, "-j list counters"):
			return []byte(rec.listOut), nil
		}
		return nil, fmt.Errorf("unexpected nft call %q", joined)
	}
	nftAvailable = func() error { return nil }
	t.Cleanup(func() { nftRunner, nftAvailable = oldRunner, oldAvail })
	return rec
}

func serveKernel(t *testing.T, payload []byte) (*httptest.Server, *int) {
	t.Helper()
	sum := sha256.Sum256(payload)
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Xray-linux-test.zip":
			downloads++
			_, _ = w.Write(payload)
		case "/Xray-linux-test.zip.dgst":
			fmt.Fprintf(w, "MD5= 00000000000000000000000000000000\nSHA2-256= %s\nSHA2-512= %s\n", hex.EncodeToString(sum[:]), strings.Repeat("ab", 64))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &downloads
}

func applyRequest(srv *httptest.Server, config string, enabled bool) ApplyRequest {
	return ApplyRequest{
		Type:    "xray",
		Version: "latest",
		Download: &Download{
			URL:         srv.URL + "/Xray-linux-{ARCH}.zip",
			ChecksumURL: srv.URL + "/Xray-linux-{ARCH}.zip.dgst",
			Archive:     "zip",
			BinName:     "xray",
			ArchMap:     map[string]string{runtime.GOARCH: "test"},
		},
		Files:   []File{{Name: "config.json", Content: config}},
		Args:    []string{"run", "-c", "{RUN_DIR}/config.json"},
		Hash:    hashString(config),
		Enabled: enabled,
		Ports:   []Port{{InboundID: 7, Port: 443, Network: "tcp"}, {InboundID: 8, Port: 8388, Network: "tcp,udp"}},
	}
}

func TestManagerLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as fake kernel")
	}
	rec := stubNft(t)
	srv, downloads := serveKernel(t, zipWith(t, map[string]string{
		"xray":        fakeKernel,
		"geoip.dat":   "geo",
		"../evil":     "nope",
		"LICENSE":     "x",
		"README.md":   "x",
		"sub/inner":   "flattened",
		"bad name!.x": "skipped",
	}))
	base := t.TempDir()
	m := NewManager(base)
	defer m.Shutdown()

	st, err := m.Apply(applyRequest(srv, `{"v":1}`, true))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !st.Running || st.Pid == 0 || st.Version != "Xray 9.9.9 (fake)" {
		t.Fatalf("state after apply: %+v", st)
	}
	if *downloads != 1 {
		t.Fatalf("downloads = %d", *downloads)
	}
	binDir := filepath.Join(base, "xray", "bin", "latest")
	for _, name := range []string{"xray", "geoip.dat", "inner"} {
		if _, err := os.Stat(filepath.Join(binDir, name)); err != nil {
			t.Fatalf("expected %s extracted: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "xray", "bin", "evil")); err == nil {
		t.Fatal("path traversal entry escaped the bin dir")
	}
	cfg, _ := os.ReadFile(filepath.Join(base, "xray", "run", "config.json"))
	if string(cfg) != `{"v":1}` {
		t.Fatalf("config = %q", cfg)
	}
	if len(rec.scripts) != 1 || !strings.Contains(rec.scripts[0], "udp dport 8388 counter name in_8_udp") {
		t.Fatalf("nft script: %v", rec.scripts)
	}

	// Same hash: no restart, no new nft script, no re-download.
	pid := st.Pid
	st, err = m.Apply(applyRequest(srv, `{"v":1}`, true))
	if err != nil || st.Pid != pid {
		t.Fatalf("unchanged apply restarted the kernel: %+v %v", st, err)
	}
	if len(rec.scripts) != 1 || *downloads != 1 {
		t.Fatalf("unchanged apply must be a no-op: scripts=%d downloads=%d", len(rec.scripts), *downloads)
	}

	// New config: restart.
	st, err = m.Apply(applyRequest(srv, `{"v":2}`, true))
	if err != nil || st.Pid == pid || !st.Running {
		t.Fatalf("changed apply must restart: %+v %v", st, err)
	}

	// Broken config: reported as error with the kernel's output.
	_, err = m.Apply(applyRequest(srv, `{"crash":true}`, true))
	if err == nil || !strings.Contains(err.Error(), "config error") {
		t.Fatalf("crashing kernel must surface its log, got %v", err)
	}

	// Disable stops the process.
	st, err = m.Apply(applyRequest(srv, `{"v":3}`, false))
	if err != nil || st.Running {
		t.Fatalf("disabled kernel still running: %+v %v", st, err)
	}

	rec.listOut = `{"nftables":[{"metainfo":{}},{"counter":{"family":"inet","name":"in_7_tcp","table":"flvx_kernel","packets":3,"bytes":1200}},{"counter":{"family":"inet","name":"out_7_tcp","table":"flvx_kernel","bytes":99}},{"counter":{"name":"in_9_tcp","table":"other","bytes":5}}]}`
	status := m.Status("xray")
	if !status.NftAvailable || len(status.Counters) != 2 || status.Epoch == "" {
		t.Fatalf("status counters: %+v", status)
	}
	if status.Counters[0] != (Counter{InboundID: 7, Proto: "tcp", Dir: "in", Bytes: 1200}) {
		t.Fatalf("counter parse: %+v", status.Counters[0])
	}
	if len(status.Kernels) != 1 || len(status.Kernels[0].Logs) == 0 {
		t.Fatalf("logs missing: %+v", status.Kernels)
	}

	// Restore on a fresh manager re-uses the installed binary.
	m2 := NewManager(base)
	defer m2.Shutdown()
	m2.Restore()
	if *downloads != 1 {
		t.Fatalf("restore must not re-download: %d", *downloads)
	}
	if st := m2.Status("").Kernels; len(st) != 1 || st[0].ConfigHash != hashString(`{"v":3}`) || st[0].Running {
		t.Fatalf("restored state: %+v", st)
	}
	if _, err := m2.Control("xray", "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if st := m2.Status("").Kernels[0]; !st.Running {
		t.Fatalf("start did not run kernel: %+v", st)
	}

	if _, err := m2.Control("xray", "remove"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "xray")); !os.IsNotExist(err) {
		t.Fatal("remove must delete kernel files")
	}
	last := rec.scripts[len(rec.scripts)-1]
	if strings.Contains(last, "chain input") {
		t.Fatalf("after removing the only kernel the table must be dropped: %s", last)
	}
}

func TestChecksumMismatchRejected(t *testing.T) {
	stubNft(t)
	srv, _ := serveKernel(t, zipWith(t, map[string]string{"xray": fakeKernel}))
	req := applyRequest(srv, `{}`, true)
	req.Download.URL = srv.URL + "/Xray-linux-{ARCH}.zip"
	req.Download.ChecksumURL = srv.URL + "/missing.dgst"
	m := NewManager(t.TempDir())
	if _, err := m.Apply(req); err == nil || !strings.Contains(err.Error(), "校验") {
		t.Fatalf("missing checksum must fail: %v", err)
	}
}

func TestApplyValidation(t *testing.T) {
	m := NewManager(t.TempDir())
	base := ApplyRequest{Type: "xray", Version: "latest", Download: &Download{BinName: "xray", Archive: "zip"}}
	cases := map[string]func(r *ApplyRequest){
		"unknown type":  func(r *ApplyRequest) { r.Type = "gost" },
		"bad version":   func(r *ApplyRequest) { r.Version = "../../etc" },
		"file escape":   func(r *ApplyRequest) { r.Files = []File{{Name: "../x"}} },
		"bad bin name":  func(r *ApplyRequest) { r.Download.BinName = "a/b" },
		"bad archive":   func(r *ApplyRequest) { r.Download.Archive = "rar" },
		"bad port":      func(r *ApplyRequest) { r.Ports = []Port{{InboundID: 1, Port: 0}} },
		"no download":   func(r *ApplyRequest) { r.Download = nil },
		"hidden config": func(r *ApplyRequest) { r.Files = []File{{Name: ".."}} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := base
			d := *base.Download
			r.Download = &d
			mutate(&r)
			if _, err := m.Apply(r); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestParseChecksum(t *testing.T) {
	h := strings.Repeat("a1", 32)
	cases := map[string]string{
		h + "  Xray-linux-64.zip\n":                                         h,
		"MD5= d41d8cd98f00b204e9800998ecf8427e\nSHA2-256= " + h + "\n":      h,
		"SHA2-512= " + strings.Repeat("cd", 64) + "\nSHA2-256= " + h + "\n": h,
	}
	for in, want := range cases {
		if got, ok := parseChecksum(in); !ok || got != want {
			t.Fatalf("parseChecksum(%q) = %q %v", in, got, ok)
		}
	}
	if _, ok := parseChecksum("nothing"); ok {
		t.Fatal("expected no checksum")
	}
}

func TestNftScriptOnlyCounts(t *testing.T) {
	script := nftScript(expandPorts([]Port{{InboundID: 2, Port: 80, Network: "tcp,udp"}, {InboundID: 1, Port: 443, Network: "tcp"}}))
	for _, bad := range []string{"drop", "reject", "policy drop"} {
		if strings.Contains(script, bad) {
			t.Fatalf("counting table must never filter: %s", script)
		}
	}
	if strings.Index(script, "in_1_tcp") > strings.Index(script, "in_2_tcp") {
		t.Fatal("rules must be sorted for a stable signature")
	}
	if !strings.HasPrefix(script, "table inet flvx_kernel {}\ndelete table inet flvx_kernel\n") {
		t.Fatalf("script must recreate the table atomically: %s", script)
	}
}

func TestProcessBackoffRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	logs := &logRing{}
	p := newProcess("/bin/sh", []string{"-c", "echo boom; exit 3"}, t.TempDir(), logs)
	p.start()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, restarts, _, _ := p.snapshot(); restarts >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.stop()
	_, _, restarts, lastErr, _ := p.snapshot()
	if restarts < 2 || !strings.Contains(lastErr, "exit status 3") {
		t.Fatalf("restarts=%d lastErr=%q", restarts, lastErr)
	}
	if !strings.Contains(strings.Join(logs.tail(0), "\n"), "boom") {
		t.Fatal("process output not captured")
	}
}
