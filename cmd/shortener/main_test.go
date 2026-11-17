package main

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPrintBuildInfo(t *testing.T) {
	tests := []struct {
		name    string
		version string
		date    string
		commit  string
		want    string
	}{
		{
			name: "unset values",
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:    "provided values",
			version: "v1.0.0",
			date:    "2026-11-15",
			commit:  "abc1234",
			want:    "Build version: v1.0.0\nBuild date: 2026-11-15\nBuild commit: abc1234\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			printBuildInfo(&stdout, tt.version, tt.date, tt.commit)
			if got := stdout.String(); got != tt.want {
				t.Fatalf("stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunStopsOnSignals(t *testing.T) {
	if os.Getenv("SHORTENER_SIGNAL_HELPER") == "1" {
		os.Args = []string{"shortener"}
		if err := run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		return
	}

	tests := []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGTERM", signal: syscall.SIGTERM},
		{name: "SIGINT", signal: syscall.SIGINT},
		{name: "SIGQUIT", signal: syscall.SIGQUIT},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("reserve address: %v", err)
			}
			address := listener.Addr().String()
			if err := listener.Close(); err != nil {
				t.Fatalf("close reserved listener: %v", err)
			}

			cmd := exec.Command(os.Args[0], "-test.run=^TestRunStopsOnSignals$")
			cmd.Env = signalHelperEnvironment(address)
			var output lockedBuffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if err := cmd.Start(); err != nil {
				t.Fatalf("start helper: %v", err)
			}
			waited := false
			t.Cleanup(func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})

			waitForHelperServer(t, address, &output)
			if err := cmd.Process.Signal(tt.signal); err != nil {
				t.Fatalf("send %s: %v", tt.name, err)
			}

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				waited = true
				if err != nil {
					t.Fatalf("helper exited with error: %v\n%s", err, output.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("helper did not stop after %s\n%s", tt.name, output.String())
			}
		})
	}
}

func signalHelperEnvironment(address string) []string {
	overrides := map[string]string{
		"SHORTENER_SIGNAL_HELPER": "1",
		"SERVER_ADDRESS":          address,
		"BASE_URL":                "http://" + address,
		"FILE_STORAGE_PATH":       "",
		"DATABASE_DSN":            "",
		"ENABLE_HTTPS":            "false",
		"AUDIT_FILE":              "",
		"AUDIT_URL":               "",
		"CONFIG":                  "",
	}

	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func waitForHelperServer(t *testing.T, address string, output *lockedBuffer) {
	t.Helper()

	client := &http.Client{
		Timeout: 50 * time.Millisecond,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + address + "/ping")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper server did not start\n%s", output.String())
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
