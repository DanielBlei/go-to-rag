package main

import (
	"os"

	"github.com/DanielBlei/go-to-rag/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
