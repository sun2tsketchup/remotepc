# Lịch sử phiên bản RemotePC

## v0.5.2 (02/11/2026)
- App Android: sửa lỗi bấm "Quét mã QR" bị văng app. Chuyển sang trình quét QR của Google, không cần cấp quyền camera; nếu máy không hỗ trợ thì báo lỗi trong app thay vì văng.

## v0.5.1 (02/11/2026)
- App Android ký bằng khoá riêng của bạn: từ bản này trở đi cài APK mới đè lên bản cũ được (riêng lần chuyển từ v0.5.0 cần gỡ app cũ 1 lần).
- GitHub tự build file exe và APK mỗi lần cập nhật, tải ở trang Releases.

## v0.5.0 (02/11/2026)
- Tuỳ chọn "Tự chạy ngầm khi bật Windows": bật máy là RemotePC chạy luôn, không hiện cửa sổ.
- Đóng cửa sổ thì RemotePC vẫn chạy ngầm, có biểu tượng ở khay đồng hồ (bấm để mở lại, chuột phải để thoát hẳn).
- Tuỳ chọn "Giữ máy tính không ngủ" (bật sẵn) để điện thoại vào được bất cứ lúc nào.
- Nhớ chế độ "Qua Internet": lần trước bật thì lần sau tự bật lại.
- Chỉ chạy 1 bản RemotePC; mở thêm lần nữa thì hiện lại cửa sổ đang chạy.

## v0.4.0 (02/11/2026)
- Nghe âm thanh máy tính trên điện thoại: nút 🔇/🔊 ở thanh trên cùng (chỉ bản Windows .exe).
- Thanh nút nhanh luôn hiện trên màn hình: Giữ chuột giữa, Ctrl, Shift, Alt (bấm để giữ, bấm lại để thả), Esc, Enter, Delete, Undo, Redo, Copy, Paste, Lưu, Alt+Tab, Desktop.
- Mã PIN được lưu lại, bật lại máy vẫn giữ mã cũ (chỉ đổi khi bấm "Đổi mã PIN").
- Điện thoại mất kết nối thì máy tính tự thả mọi phím/nút đang giữ.

## v0.3.0 (02/11/2026)
- Cửa sổ phần mềm riêng trên Windows (mã QR, PIN, Đổi PIN, Huỷ ghép nối, Thoát), bỏ cửa sổ đen và trang web.
- Chế độ "Qua Internet" (Cloudflare), link đổi mỗi lần bật.
- Chuột rời trên máy tính bảng: trái/phải/giữa, giữ và kéo, con lăn mịn.
- Bàn phím rời gửi thẳng từng phím (giữ Shift/Ctrl được).
- Chọn màn hình nào thì con trỏ chuyển sang và chỉ chạy trong màn hình đó.
- Hiện số phiên bản trên cửa sổ PC, trong ⚙️ của app điện thoại và trong tên file.

## v0.2.1 (02/11/2026)
- Mũi tên chuột hiện trên điện thoại; chạm không làm con trỏ nhảy, kéo ngón tay thì con trỏ đi theo đúng quãng.
- Nút MH1 / MH2 / Tất cả ở thanh trên cùng.
- Nút "Giữ chuột" thay cho chế độ Chạm.

## v0.2.0 (02/11/2026)
- Bản RemotePC.exe một file, không cần cài Python (viết lại bằng Go).

## v0.1.0 (02/11/2026)
- Bản đầu: agent Python + app điện thoại chạy trên trình duyệt, xem màn hình, touchpad, bàn phím, media, ghép nối PIN.
