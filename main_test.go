// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"golang.org/x/tools/imports"
)

type testCase struct {
	Name  string
	In    string
	Out   string
	NSOut string
}

var testCases = []testCase{
	{
		Name: "no change - only net",
		In: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	sub(&net.UDPAddr{ip2, 12345})
	c := &net.TCPAddr{IP: ip3, Port: 54321}
	d := &net.TCPAddr{ip4, 0}
	p := 1234
	e := &net.TCPAddr{ip4, p}
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	sub(&net.UDPAddr{ip2, 12345})
	c := &net.TCPAddr{IP: ip3, Port: 54321}
	d := &net.TCPAddr{ip4, 0}
	p := 1234
	e := &net.TCPAddr{ip4, p}
	return &net.TCPAddr{ip5}, nil
}
`,
	},
	{
		Name: "change net.ParseIP",
		In: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := net.ParseIP(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import (
	"net"

	netutils "k8s.io/utils/net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
	},
	{
		Name: "change net.ParseIP and ParseCIDR",
		In: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := net.ParseIP(addr)
	d, _, err := net.ParseCIDR(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import (
	"net"

	netutils "k8s.io/utils/net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	d, _, err := netutils.ParseCIDRSloppy(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
	},
	{
		Name: "change net.ParseIP and ParseCIDR and remove net",
		In: `package main

import "net"

func f() {
	addr := "1.2.3.4"
	c := net.ParseIP(addr)
	d, _, err := net.ParseCIDR(addr)
}
`,
		Out: `package main

import (
	netutils "k8s.io/utils/net"
)

func f() {
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	d, _, err := netutils.ParseCIDRSloppy(addr)
}
`,
	},
	{
		Name: "existing netutils and change net.ParseIP and ParseCIDR and remove net",
		In: `package main

import (
	"net"

	utilnet "k8s.io/utils/net"
)

func f() {
	addr := "1.2.3.4"
	c := net.ParseIP(addr)
	d, _, err := net.ParseCIDR(addr)
	utilnet.IsIPv6(d)
}
`,
		Out: `package main

import (
	netutils "k8s.io/utils/net"
)

func f() {
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	d, _, err := netutils.ParseCIDRSloppy(addr)
	netutils.IsIPv6(d)
}
`,
	},
	{
		Name: "no change when parsing constant string",
		In: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	c := net.ParseIP("1.2.3.4")
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import "net"

func f() net.Addr {
	a := &net.IPAddr{ip1}
	c := net.ParseIP("1.2.3.4")
	return &net.TCPAddr{ip5}, nil
}
`,
	},
	{
		Name: "rewrite constant string ParseIPSloppy to ParseIP",
		In: `package main

import (
	"net"

	netutils "k8s.io/utils/net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	c := netutils.ParseIPSloppy("1.2.3.4")
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import (
	"net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	c := net.ParseIP("1.2.3.4")
	return &net.TCPAddr{ip5}, nil
}
`,
		NSOut: `package main

import (
	"net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	c := net.ParseIP("1.2.3.4")
	return &net.TCPAddr{ip5}, nil
}
`,
	},
	{
		Name: "already correctly uses ParseIPSloppy",
		In: `package main

import (
	"net"

	netutils "k8s.io/utils/net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
		Out: `package main

import (
	"net"

	netutils "k8s.io/utils/net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := netutils.ParseIPSloppy(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
		NSOut: `package main

import (
	"net"
)

func f() net.Addr {
	a := &net.IPAddr{ip1}
	addr := "1.2.3.4"
	c := net.ParseIP(addr)
	return &net.TCPAddr{ip5}, nil
}
`,
	},
}

func fnop(*ast.File) bool { return false }

func parseFixPrint(t *testing.T, desc, in string, nonSloppy, mustBeGofmt bool) (out string, fixed, ok bool) {
	file, err := parser.ParseFile(fset, desc, in, parserMode)
	if err != nil {
		t.Errorf("parsing: %v", err)
		return
	}

	outb, err := gofmtFile(file)
	if err != nil {
		t.Errorf("printing: %v", err)
		return
	}
	if s := string(outb); in != s && mustBeGofmt {
		t.Errorf("not gofmt-formatted.\n--- %s\n%s\n--- %s | gofmt\n%s",
			desc, in, desc, s)
		tdiff(t, in, s)
		return
	}

	fixed = sloppyParsers(file, nonSloppy)

	outb, err = gofmtFile(file)
	if err != nil {
		t.Errorf("printing: %v", err)
		return
	}

	outc, err := imports.Process("", outb, nil)
	if err != nil {
		t.Errorf("printing: %v", err)
		return
	}

	return string(outc), fixed, true
}

func TestRewrite(t *testing.T) {
	for _, tt := range testCases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			// Apply fix: should get tt.Out.
			out, fixed, ok := parseFixPrint(t, tt.Name, tt.In, false, true)
			if !ok {
				return
			}

			// reformat to get printing right
			out, _, ok = parseFixPrint(t, tt.Name, out, false, false)
			if !ok {
				return
			}

			if out != tt.Out {
				t.Errorf("incorrect output.\n")
				if !strings.HasPrefix(tt.Name, "testdata/") {
					t.Errorf("--- have\n%s\n--- want\n%s", out, tt.Out)
				}
				tdiff(t, out, tt.Out)
				return
			}

			if changed := out != tt.In; changed != fixed {
				t.Errorf("changed=%v != fixed=%v", changed, fixed)
				return
			}

			// Should not change if run again.
			out2, fixed2, ok := parseFixPrint(t, tt.Name+" output", out, false, true)
			if !ok {
				return
			}

			if fixed2 {
				t.Errorf("applied fixes during second round")
				return
			}

			if out2 != out {
				t.Errorf("changed output after second round of fixes.\n--- output after first round\n%s\n--- output after second round\n%s",
					out, out2)
				tdiff(t, out, out2)
			}
		})
	}
}

func TestPreferNonSloppy(t *testing.T) {
	for _, tt := range testCases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			// Apply fix: should get tt.Out.
			out, fixed, ok := parseFixPrint(t, tt.Name, tt.In, true, true)
			if !ok {
				return
			}

			// reformat to get printing right
			out, _, ok = parseFixPrint(t, tt.Name, out, true, false)
			if !ok {
				return
			}

			nonSloppyOut := tt.NSOut
			if nonSloppyOut == "" {
				nonSloppyOut = tt.In
			}

			if out != nonSloppyOut {
				t.Errorf("incorrect output.\n")
				if !strings.HasPrefix(tt.Name, "testdata/") {
					t.Errorf("--- have\n%s\n--- want\n%s", out, nonSloppyOut)
				}
				tdiff(t, out, nonSloppyOut)
				return
			}

			if changed := out != tt.In; changed != fixed {
				t.Errorf("changed=%v != fixed=%v", changed, fixed)
				return
			}

			// Should not change if run again.
			out2, fixed2, ok := parseFixPrint(t, tt.Name+" output", out, true, true)
			if !ok {
				return
			}

			if fixed2 {
				t.Errorf("applied fixes during second round")
				return
			}

			if out2 != out {
				t.Errorf("changed output after second round of fixes.\n--- output after first round\n%s\n--- output after second round\n%s",
					out, out2)
				tdiff(t, out, out2)
			}
		})
	}
}

func tdiff(t *testing.T, a, b string) {
	data, err := Diff("go-fix-test", []byte(a), []byte(b))
	if err != nil {
		t.Error(err)
		return
	}
	t.Error(string(data))
}
