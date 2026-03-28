//go:build !linux

package bench

import "fmt"

func dropFileCache(path string) error {
	return fmt.Errorf("cold cache enforcement is unsupported on %s for %s", runtimeGOOS(), path)
}

func runtimeGOOS() string {
	return "non-linux"
}
