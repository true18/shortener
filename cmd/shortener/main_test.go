package main

import (
	"bytes"
	"testing"
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
