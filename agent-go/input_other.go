//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

const inputAvailable = false

func ensureOn(m Monitor)          {}
func initPlatform()               {}
func cursorPos() (int, int, bool) { return 0, 0, false }
func handleInput(m msg, mon Monitor) error {
	return errors.New("không hỗ trợ điều khiển trên hệ điều hành này")
}

func hideConsole(cmd *exec.Cmd) {}
