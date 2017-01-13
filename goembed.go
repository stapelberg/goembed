// goembed generates a Go source file from an input file.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"text/template"
	"unicode/utf8"
)

var (
	packageFlag = flag.String("package", "", "Go package name")
	varFlag     = flag.String("var", "", "Go var name")
	gzipFlag    = flag.Bool("gzip", false, "Whether to gzip contents")
)

func main() {
	flag.Parse()

	raw, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Reading stdin: %v", err)
	}

	fmt.Printf("package %s\n\n", *packageFlag)

	// Generate []byte(<big string constant>) instead of []byte{<list of byte values>}.
	// The latter causes a memory explosion in the compiler (60 MB of input chews over 9 GB RAM).
	// Doing a string conversion avoids some of that, but incurs a slight startup cost.
	if !*gzipFlag {
		fmt.Printf(`var %s = []byte("`, *varFlag)
	} else {
		var buf bytes.Buffer
		gzw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if _, err := gzw.Write(raw); err != nil {
			log.Fatal(err)
		}
		if err := gzw.Close(); err != nil {
			log.Fatal(err)
		}
		gz := buf.Bytes()

		if err := gzipPrologue.Execute(os.Stdout, *varFlag); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("var %s []byte // set in init\n\n", *varFlag)
		fmt.Printf(`var %s_gzip = []byte("`, *varFlag)
		raw = gz
	}

	io.Copy(&writer{w: os.Stdout}, bytes.NewReader(raw))
	fmt.Println(`")`)
}

type writer struct {
	w io.Writer
}

func (w *writer) Write(data []byte) (n int, err error) {
	n = len(data)

	for err == nil && len(data) > 0 {
		// https://golang.org/ref/spec#String_literals: "Within the quotes, any
		// character may appear except newline and unescaped double quote. The
		// text between the quotes forms the value of the literal, with backslash
		// escapes interpreted as they are in rune literals […]."
		switch b := data[0]; b {
		case '\\':
			_, err = w.w.Write([]byte(`\\`))
		case '"':
			_, err = w.w.Write([]byte(`\"`))
		case '\n':
			_, err = w.w.Write([]byte(`\n`))

		case '\x00':
			// https://golang.org/ref/spec#Source_code_representation: "Implementation
			// restriction: For compatibility with other tools, a compiler may
			// disallow the NUL character (U+0000) in the source text."
			_, err = w.w.Write([]byte(`\x00`))

		default:
			// https://golang.org/ref/spec#Source_code_representation: "Implementation
			// restriction: […] A byte order mark may be disallowed anywhere else in
			// the source."
			const byteOrderMark = '\uFEFF'

			if r, size := utf8.DecodeRune(data); r != utf8.RuneError && r != byteOrderMark {
				_, err = w.w.Write(data[:size])
				data = data[size:]
				continue
			}

			_, err = fmt.Fprintf(w.w, `\x%02x`, b)
		}
		data = data[1:]
	}

	return n - len(data), err
}

var gzipPrologue = template.Must(template.New("").Parse(`
import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
)

func init() {
	r, err := gzip.NewReader(bytes.NewReader({{.}}_gzip))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	{{.}}, err = ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
}
`))
