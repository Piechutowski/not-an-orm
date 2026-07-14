// Command edbml-ls is the EDBML language server. It speaks the Language
// Server Protocol over stdin/stdout; editors launch it, humans rarely do.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Piechutowski/not-an-orm/edbml/lsp"
)

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Println("edbml-ls", lsp.Version)
		return
	}
	if err := lsp.NewServer().RunStdio(); err != nil {
		log.SetOutput(os.Stderr)
		log.Fatal(err)
	}
}
