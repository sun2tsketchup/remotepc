//go:build !windows

package main

import (
	"fmt"
	"strings"
)

func fatalUI(s string) { fmt.Println(s) }

func ensureSingleInstance() {}

// runUI trên macOS/Linux: in thông tin ra dòng lệnh rồi chờ.
func runUI(app *App) {
	line := strings.Repeat("=", 56)
	fmt.Println(line)
	fmt.Println("  RemotePC đang chạy. Trên điện thoại (cùng Wi-Fi) mở:")
	for _, u := range app.URLs {
		fmt.Println("     " + u)
	}
	fmt.Println("  Mã PIN ghép nối: " + app.Auth.PIN())
	if !inputAvailable {
		fmt.Println("  [!] Không điều khiển được chuột/bàn phím trên hệ điều hành này, chỉ xem màn hình.")
	}
	fmt.Println(line)
	select {}
}
