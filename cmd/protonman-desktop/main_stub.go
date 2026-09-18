//go:build !desktop

package main

import "log"

func runDesktop() {
	log.Fatal("protonman-desktop requires the desktop build tag")
}
