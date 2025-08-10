package main

import (
	"fmt"
	"log"
	"os"

	"github.com/wozozo/s3pit/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		log.Fatal(err)
	}
}
