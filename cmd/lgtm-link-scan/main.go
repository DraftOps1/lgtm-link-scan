package main

import (
	"log"

	"github.com/DraftOps1/lgtm-link-scan/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Fatal(err)
	}
}
