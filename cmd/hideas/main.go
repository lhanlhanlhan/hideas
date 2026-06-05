package main

import (
	"os"

	"hideas/internal/hideas"
)

func main() {
	os.Exit(hideas.Run(os.Args[1:], os.Stdout, os.Stderr))
}
