// Package noosexit provides an analyzer that discourages os.Exit in main.
package noosexit

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports direct calls to os.Exit from main in a main package.
var Analyzer = &analysis.Analyzer{
	Name: "noosexit",
	Doc:  "reports direct calls to os.Exit from main functions",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "main" || fn.Body == nil {
				continue
			}

			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Exit" {
					return true
				}

				function, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
				if ok && function.Name() == "Exit" && function.Pkg() != nil && function.Pkg().Path() == "os" {
					pass.Reportf(call.Pos(), "direct calls to os.Exit are not allowed in main; return an error instead")
				}
				return true
			})
		}
	}

	return nil, nil
}
