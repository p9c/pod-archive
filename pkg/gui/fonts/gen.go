// Copyright 2016 The Go Authors. All rights reserved. Use of this source code is governed by a BSD-style license that
// can be found in the LICENSE file.

// this will run this generator if go generate is called on this directory
//go:generate go run gen.go log.go

package main

// This program generates the subdirectories of Go packages that contain []byte versions of the TrueType font files
// under ./ttfs.
//
// Currently, "go run gen.go" needs to be run manually. This isn't done by the usual "go generate" mechanism as there
// isn't any other Go code in this directory (excluding sub-directories) to attach a "go:generate" line to.
//
// In any case, code generation should only need to happen when the underlying TTF files change, which isn't expected to
// happen frequently.

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const suffix = ".ttf"

func main() {
	ttfs, err := os.Open("ttfs")
	if err != nil {
		Fatal(err)
	}
	defer ttfs.Close()

	infos, err := ttfs.Readdir(-1)
	if err != nil {
		Fatal(err)
	}
	for _, info := range infos {
		ttfName := info.Name()
		if !strings.HasSuffix(ttfName, suffix) {
			continue
		}
		do(ttfName)
	}
}

func do(ttfName string) {
	fontName := fontName(ttfName)
	pkgName := pkgName(ttfName)
	if err := os.Mkdir(pkgName, 0777); err != nil && !os.IsExist(err) {
		Fatal(err)
	}
	src, err := ioutil.ReadFile(filepath.Join("ttfs", ttfName))
	if err != nil {
		Fatal(err)
	}

	// desc := "a proportional-width, sans-serif"
	// if strings.Contains(ttfName, "Mono") {
	// 	desc = "a fixed-width, slab-serif"
	// }

	b := new(bytes.Buffer)
	fmt.Fprintf(b, "// generated by go run github.com/p9c/pod/pkg/gui/fonts; DO NOT EDIT\n\n")
	fmt.Fprintf(b, "package %s\n\n", pkgName)
	fmt.Fprintf(b, "// TTF is the data for the %q TrueType font.\n", fontName)
	fmt.Fprintf(b, "var TTF = []byte{")
	for i, x := range src {
		if i&15 == 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(b, "%#02x,", x)
	}
	fmt.Fprintf(b, "\n}\n")

	dst, err := format.Source(b.Bytes())
	if err != nil {
		Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(pkgName, "data.go"), dst, 0666); err != nil {
		Fatal(err)
	}
}

// fontName maps "Go-Regular.ttf" to "Go Regular".
func fontName(ttfName string) string {
	s := ttfName[:len(ttfName)-len(suffix)]
	s = strings.Replace(s, "-", " ", -1)
	return s
}

// pkgName maps "Go-Regular.ttf" to "goregular".
func pkgName(ttfName string) string {
	s := ttfName[:len(ttfName)-len(suffix)]
	s = strings.Replace(s, "-", "", -1)
	s = strings.ToLower(s)
	return s
}
