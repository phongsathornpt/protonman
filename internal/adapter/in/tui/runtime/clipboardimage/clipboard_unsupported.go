//go:build !darwin && !linux

package clipboardimage

import "fmt"

type Result struct {
	Path   string
	Width  int
	Height int
	Err    error
}

func Load() Result {
	return Result{Err: fmt.Errorf("clipboard image paste is not supported on this platform")}
}

func WriteTempPNG(_ []byte) (string, error) {
	return "", fmt.Errorf("clipboard image paste is not supported on this platform")
}
