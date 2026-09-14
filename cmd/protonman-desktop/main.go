//go:build desktop

package main

import (
	"context"
	"log"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop"
)

func main() {
	if err := desktop.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
