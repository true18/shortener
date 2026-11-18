package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFinishExitCodeAndStderr(t *testing.T) {
	if mode := os.Getenv("SHORTENER_FINISH_HELPER"); mode != "" {
		if mode == "error" {
			finish(errors.New("startup failed"))
			return
		}

		finish(nil)
		return
	}

	tests := []struct {
		name       string
		mode       string
		wantCode   int
		wantStderr string
	}{
		{name: "success", mode: "success", wantCode: 0},
		{name: "startup error", mode: "error", wantCode: 1, wantStderr: "startup failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestFinishExitCodeAndStderr$")
			command.Env = append(os.Environ(), "SHORTENER_FINISH_HELPER="+tt.mode)
			var stderr bytes.Buffer
			command.Stderr = &stderr

			err := command.Run()
			gotCode := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run returned %v", err)
				}
				gotCode = exitErr.ExitCode()
			}

			if gotCode != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr = %q", gotCode, tt.wantCode, stderr.String())
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
