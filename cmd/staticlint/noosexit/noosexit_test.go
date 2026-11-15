package noosexit_test

import (
	"testing"

	"github.com/true18/shortener/cmd/staticlint/noosexit"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), noosexit.Analyzer,
		"mainpkg",
		"aliaspkg",
		"helperpkg",
		"otherpkg",
		"customexitpkg",
		"closurepkg",
	)
}
