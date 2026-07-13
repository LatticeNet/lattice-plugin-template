package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/LatticeNet/lattice-plugin-template/tools/pluginpack"
)

func main() {
	source := flag.String("source", "", "source bundle directory")
	output := flag.String("output", "", "output .tar.gz path")
	flag.Parse()

	if *source == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "usage: pluginpack -source <dir> -output <path>")
		os.Exit(2)
	}

	digest, err := pluginpack.PackFile(*source, *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pluginpack: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(digest)
}
