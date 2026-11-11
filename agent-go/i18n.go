package main

// Ngôn ngữ giao diện trên máy tính: mặc định tiếng Việt, có tiếng Anh.

import "strings"

var lang = "vi"

var enText = map[string]string{
	"Cùng Wi-Fi":                   "Same Wi-Fi",
	"Qua Internet":                 "Via Internet",
	"Tự chạy ngầm khi bật Windows": "Run in background when Windows starts",
	"Giữ máy tính không ngủ để điện thoại vào bất cứ lúc nào": "Keep this PC awake so the phone can connect any time",
	"Đổi mã PIN":                  "New PIN",
	"Huỷ ghép nối":                "Unpair all",
	"Thoát hẳn":                   "Quit",
	"Sao chép link":               "Copy link",
	"Tắt Internet":                "Turn off Internet",
	"Thử lại":                     "Retry",
	"Đang chạy":                   "Running",
	"%d thiết bị đang điều khiển": "%d device(s) connected",
	"Điện thoại dùng chung Wi-Fi với máy tính này":        "Phone must be on the same Wi-Fi as this PC",
	"Dùng được ở bất cứ đâu có Internet (4G, Wi-Fi khác)": "Works anywhere with Internet (4G, other Wi-Fi)",
	"Đang tải thành phần kết nối (chỉ lần đầu)…":          "Downloading connection component (first time only)…",
	"Đang tạo đường truyền qua Internet…":                 "Creating Internet connection…",
	"Quét mã QR bằng camera điện thoại để kết nối":        "Scan the QR code with your phone camera to connect",
	"MÃ PIN": "PIN",
	"%d điện thoại đã ghép nối  ·  Đóng cửa sổ: vẫn chạy ngầm ở khay đồng hồ": "%d paired phone(s)  ·  Closing the window keeps it running in the tray",
	"Không bật được tự chạy: ":                          "Could not enable auto start: ",
	"Đã sao chép link:\n":                               "Link copied:\n",
	"\n\nGửi link này sang điện thoại rồi nhập mã PIN.": "\n\nSend this link to your phone, then enter the PIN.",
	"Huỷ ghép nối tất cả điện thoại?\nLần sau điện thoại phải nhập lại mã PIN.":                                        "Unpair all phones?\nPhones will need the PIN again next time.",
	"Thoát hẳn Betagon Remote PC?\nĐiện thoại sẽ không vào được máy tính cho tới khi mở lại.":                          "Quit Betagon Remote PC?\nPhones cannot connect until you open it again.",
	"Betagon Remote PC vẫn đang chạy ngầm":                                                                             "Betagon Remote PC is still running",
	"Điện thoại vẫn vào được. Bấm biểu tượng Betagon Remote PC ở khay đồng hồ để mở lại, bấm chuột phải để thoát hẳn.": "Phones can still connect. Click the tray icon to open it again, right-click to quit.",
	"Không tạo được cửa sổ Betagon Remote PC":                                                                          "Could not create the Betagon Remote PC window",
	" đang chạy ngầm":                                      " running in background",
	"Mở Betagon Remote PC":                                 "Open Betagon Remote PC",
	"Thoát hẳn Betagon Remote PC":                          "Quit Betagon Remote PC",
	"Không tải được cloudflared: ":                         "Could not download cloudflared: ",
	"Hãy cài cloudflared trước (brew install cloudflared)": "Please install cloudflared first (brew install cloudflared)",
	"Không chạy được cloudflared: ":                        "Could not start cloudflared: ",
	"Hết thời gian chờ tạo đường truyền, kiểm tra Internet rồi thử lại": "Timed out creating the connection, check the Internet and retry",
	"cloudflared đã dừng (%v)":                                              "cloudflared stopped (%v)",
	"Mất đường truyền Internet (%v), bấm để thử lại":                        "Internet connection lost (%v), click to retry",
	"Không mở được cổng %d.\nCó thể Betagon Remote PC đang chạy rồi.\n\n%v": "Cannot open port %d.\nBetagon Remote PC may already be running.\n\n%v",
}

// tr dịch một câu sang ngôn ngữ đang chọn (câu gốc là tiếng Việt).
func tr(s string) string {
	if lang == "en" {
		if t, ok := enText[s]; ok {
			return t
		}
	}
	return s
}

func setLang(l string) {
	if strings.HasPrefix(l, "en") {
		lang = "en"
	} else {
		lang = "vi"
	}
}
