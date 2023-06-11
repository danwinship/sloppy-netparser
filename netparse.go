package main

import (
	"go/ast"
	"go/token"
)

func sloppyParsers(f *ast.File) bool {
	if ok, _ := getImport(f, "net"); !ok {
		return false
	}

	addNetUtils, addNet := false, false
	walk(f, func(n interface{}) {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		wantNonSloppy := false
		if len(ce.Args) == 1 {
			if bl, ok := ce.Args[0].(*ast.BasicLit); ok {
				wantNonSloppy = bl.Kind == token.STRING
			}
		}

		se, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok || se.Sel == nil {
			return
		}
		ss := se.Sel.String()

		if isTopName(se.X, "net") && !wantNonSloppy {
			switch ss {
			case "ParseIP":
				id, _ := se.X.(*ast.Ident)
				id.Name = "netutils"
				se.Sel.Name = "ParseIPSloppy"
				addNetUtils = true
			case "ParseCIDR":
				se.X.(*ast.Ident).Name = "netutils"
				se.Sel.Name = "ParseCIDRSloppy"
				addNetUtils = true
			}
		} else if (isTopName(se.X, "utilnet") || isTopName(se.X, "utilsnet") || isTopName(se.X, "netutil") || isTopName(se.X, "netutils")) && wantNonSloppy {
			switch ss {
			case "ParseIPSloppy":
				id, _ := se.X.(*ast.Ident)
				id.Name = "net"
				se.Sel.Name = "ParseIP"
				addNet = true
			case "ParseCIDRSloppy":
				se.X.(*ast.Ident).Name = "net"
				se.Sel.Name = "ParseCIDR"
				addNet = true
			}
		}
	})
	if addNetUtils {
		addImport(f, "netutils", "k8s.io/utils/net")
		rewriteImportName(f, "k8s.io/utils/net", "netutils", "k8s.io/utils/net")
	}
	if addNet {
		addImport(f, "", "net")
	}
	return addNetUtils || addNet
}
