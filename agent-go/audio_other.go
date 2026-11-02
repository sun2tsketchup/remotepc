//go:build !windows

package main

import "errors"

func captureLoop(stop chan struct{}, out func([]byte), errc chan error) {
	errc <- errors.New("chỉ hỗ trợ trên Windows")
}
