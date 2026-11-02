# RemotePC: dùng điện thoại điều khiển máy tính (bản MVP)

## Ý tưởng

Máy tính chạy một chương trình nhỏ (**agent**). Điện thoại (iPhone hoặc Android) chỉ cần mở trình duyệt vào địa chỉ của máy tính trong cùng mạng Wi-Fi, không phải cài app từ App Store / Google Play. Có thể "Thêm vào màn hình chính" để dùng như một app thật, toàn màn hình.

```
 Điện thoại (Safari / Chrome)            Máy tính (Windows / macOS / Linux)
 ┌──────────────────────────┐   Wi-Fi    ┌───────────────────────────────┐
 │ Web app (PWA)            │  WebSocket │ agent/server.py (Python)      │
 │ - xem màn hình (JPEG)    │ ◄───────── │ - chụp màn hình (mss)         │
 │ - touchpad, chạm-click   │ ─────────► │ - giả lập chuột/phím (pynput) │
 │ - bàn phím, phím tắt     │   lệnh     │ - ghép nối bằng PIN + token   │
 └──────────────────────────┘            └───────────────────────────────┘
```

Vì sao chọn cách này cho bản đầu: một bộ code chạy được cả iPhone lẫn Android, không cần tài khoản nhà phát triển Apple (99 USD/năm) hay duyệt app, sửa giao diện chỉ cần sửa 1 file.

## Tính năng

- **Xem màn hình trực tiếp**, có mũi tên chuột hiện trên điện thoại, nút **MH1 / MH2 / Tất cả** ở thanh trên cùng để chuyển màn hình hoặc xem mọi màn hình cùng lúc, 3 mức chất lượng (Tiết kiệm / Vừa / Nét). Hình chỉ gửi khi có thay đổi, và điện thoại xác nhận từng khung nên không bị trễ dồn.
- **Điều khiển kiểu touchpad**: chạm ở đâu cũng được, con trỏ không nhảy tới ngón tay mà đi theo hướng kéo (đúng bằng quãng ngón tay di, chỉnh tốc độ trong ⚙️) nên ngón tay không che con trỏ. Chạm = click trái, chạm 2 ngón = click phải, kéo 2 ngón = cuộn, giữ 1 ngón rồi kéo = kéo thả, nút **Giữ chuột** để kéo thả dài. Nút **1x/2x/3x** phóng to và tự bám theo con trỏ.
- **Chuột và bàn phím rời** (máy tính bảng, laptop): trỏ tới đâu chuột máy tính tới đó, đủ trái/phải/giữa, giữ và kéo (xoay/pan trong SketchUp), con lăn mịn. Bàn phím rời gửi thẳng từng phím, giữ Shift/Ctrl được. Muốn gõ tiếng Việt bằng bàn phím rời: để bàn phím máy tính bảng ở chế độ tiếng Anh và bật Unikey/EVKey trên máy tính.
- Chọn màn hình nào thì con trỏ tự chuyển sang màn hình đó và chỉ chạy trong màn hình đó.
- **Bàn phím**: gõ trực tiếp (hỗ trợ tiếng Việt có dấu), phím Ctrl/Alt/Shift/Win dạng bật-tắt, phím mũi tên, Esc, Tab, F5, F11, nút nhanh Copy / Paste / Undo / Alt+Tab / Đóng cửa sổ / Ra Desktop. Trên Mac tự đổi Ctrl thành Cmd.
- **Âm thanh**: nút 🔇/🔊 trên thanh trên cùng để nghe tiếng máy tính trên điện thoại (bản Windows). iPhone cần tắt chế độ im lặng.
- **Thanh nút nhanh** ngay dưới màn hình: giữ chuột giữa, giữ Ctrl/Shift/Alt, và các phím tắt hay dùng (vuốt ngang để xem thêm).
- **Media**: tăng/giảm/tắt âm lượng, phát/dừng, bài trước/sau, PgUp/PgDn (điều khiển slide thuyết trình).
- **Tắt truyền hình** để tiết kiệm pin/mạng, chỉ dùng như chuột + bàn phím không dây.
- **Bảo mật**: lần đầu phải nhập PIN 6 số hiện trên máy tính (hoặc quét QR). Sai 5 lần bị khoá 60 giây. Ghép xong, điện thoại giữ một token ngẫu nhiên nên lần sau vào thẳng.

## Cài và chạy

### 1. Trên máy tính

**Windows (cách nhanh nhất, không cần cài gì):** bấm đúp `RemotePC-v0.5.0.exe` (số sau chữ v là phiên bản, xem `CHANGELOG.md`). Cửa sổ phần mềm RemotePC hiện mã QR, mã PIN và địa chỉ.
- Lần đầu Windows có thể hiện "Windows protected your PC" (vì file chưa ký số): bấm **More info → Run anyway**.
- Khi Windows Firewall hỏi, chọn **Allow** (mạng Private).
- Nút **Cùng Wi-Fi / Qua Internet**: chọn "Qua Internet" để điều khiển từ 4G hay mạng khác. RemotePC tự tải thành phần kết nối của Cloudflare (lần đầu, khoảng 50 MB) và tạo link https riêng. Link này đổi mỗi lần bật lại.
- Tick **Tự chạy ngầm khi bật Windows** để bật máy là RemotePC chạy sẵn. Đóng cửa sổ thì RemotePC vẫn chạy ngầm ở khay đồng hồ (bấm biểu tượng để mở lại, chuột phải → Thoát hẳn). **Giữ máy tính không ngủ** bật sẵn để điện thoại vào lúc nào cũng được.
- Nút **Đổi mã PIN**, **Huỷ ghép nối** (bắt mọi điện thoại nhập lại PIN), **Thoát hẳn**.
- Mã nguồn bản .exe nằm ở `agent-go/` (viết bằng Go), build lại bằng `agent-go/build.sh`.

**Cách khác (Windows/macOS/Linux, dùng Python):** cần Python 3.9 trở lên ([tải tại python.org](https://www.python.org/downloads/), trên Windows nhớ tick **Add python.exe to PATH**).

- **Windows**: bấm đúp `Chay-tren-Windows.bat`.
- **macOS / Linux**: mở Terminal trong thư mục này, chạy `./chay-tren-mac-linux.sh`.

Lần đầu sẽ tự cài thư viện (mất khoảng 1 phút). Cửa sổ sẽ hiện địa chỉ dạng `http://192.168.1.10:8765`, **mã PIN** và một **mã QR**.

Lưu ý theo hệ điều hành:
- **Windows**: khi Windows Firewall hỏi, chọn **Allow** cho mạng Private.
- **macOS**: vào *System Settings → Privacy & Security*, cấp quyền **Screen Recording** và **Accessibility** cho Terminal (hoặc Python), rồi chạy lại.
- **Linux**: cần phiên X11 (Wayland chưa hỗ trợ chụp màn hình/giả lập chuột qua thư viện này).

### 2. Trên điện thoại
1. Kết nối cùng Wi-Fi với máy tính.
2. Quét mã QR bằng camera (hoặc mở địa chỉ trên bằng Safari/Chrome rồi nhập PIN).
3. Nên xoay ngang điện thoại. Để dùng như app: **iPhone**: nút Chia sẻ → *Thêm vào MH chính*; **Android**: menu ⋮ → *Thêm vào màn hình chính*.

Tuỳ chọn: `--port 9000` để đổi cổng, `--reset` để xoá mọi điện thoại đã ghép nối.

## Cấu trúc code

| File | Vai trò |
|---|---|
| `RemotePC-v*.exe` + `agent-go/` | Bản Windows 1 file, viết bằng Go (không cần Python) |
| `agent/server.py` | Server aiohttp: ghép nối, WebSocket, chụp màn hình, điều khiển chuột/phím |
| `agent/web/index.html` | Toàn bộ app điện thoại (HTML + CSS + JS, không cần build) |
| `agent/web/manifest.json` | Cho phép cài như app (PWA) |

Giao thức WebSocket (JSON từ điện thoại): `move{dx,dy}`, `moveto{x,y}` (0..1), `click{b,n}`, `down/up{b}`, `scroll{dx,dy}`, `text{s}`, `key{k}`, `combo{k:[..]}`, `cfg{mon,max_w,q,fps,stream}`, `ack`. Máy tính gửi `hello`, `frame` (kèm vị trí con trỏ) rồi ảnh JPEG dạng nhị phân.

## Giới hạn hiện tại và hướng phát triển

- Kết nối là HTTP trong mạng LAN, chưa mã hoá. Chỉ dùng ở Wi-Fi nhà/công ty tin cậy. Bước tiếp: HTTPS tự ký + ghim chứng chỉ.
- Chưa điều khiển được từ ngoài mạng nhà (4G). Bước tiếp: dùng Tailscale/ZeroTier (nhanh nhất), hoặc tự làm máy chủ chuyển tiếp + WebRTC.
- Stream JPEG phù hợp làm việc văn phòng; xem video/chơi game cần chuyển sang H.264 qua WebRTC.
- Chưa có: truyền âm thanh, gửi file, đồng bộ clipboard, chạy nền khi khởi động Windows, file .exe đóng gói sẵn (PyInstaller).
- Màn hình UAC/Ctrl+Alt+Del của Windows không điều khiển được khi agent chạy quyền thường.
