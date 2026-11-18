package main

import (
	"strings"
	"testing"
)

func TestProjectAnalyzersIncludeRequiredGroups(t *testing.T) {
	seen := make(map[string]bool)
	saCount := 0
	for _, analyzer := range projectAnalyzers() {
		if seen[analyzer.Name] {
			t.Fatalf("duplicate analyzer %q", analyzer.Name)
		}
		seen[analyzer.Name] = true
		if strings.HasPrefix(analyzer.Name, "SA") {
			saCount++
		}
	}
	if saCount == 0 {
		t.Fatal("no Staticcheck SA analyzers registered")
	}
	t.Logf("registered %d Staticcheck SA analyzers", saCount)
	for _, name := range []string{"S1000", "bodyclose", "nilerr", "noosexit", "printf", "copylocks"} {
		if !seen[name] {
			t.Errorf("analyzer %q is not registered", name)
		}
	}
}
