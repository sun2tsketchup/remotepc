"""RemotePC agent: chạy trên máy tính, cho phép điện thoại điều khiển qua mạng LAN.

Chạy:  python server.py            (cổng mặc định 8765)
       python server.py --port 9000 --reset   (đổi cổng, xoá các điện thoại đã ghép nối)
"""
import argparse
import asyncio
import io
import json
import os
import platform
import secrets
import socket
import sys
import time
from pathlib import Path

from aiohttp import web, WSMsgType

# Windows: bật DPI awareness để toạ độ chuột khớp với ảnh chụp màn hình.
if platform.system() == "Windows":
    try:
        import ctypes
        ctypes.windll.shcore.SetProcessDpiAwareness(2)
    except Exception:
        pass

import mss
from PIL import Image

WEB_DIR = Path(__file__).parent / "web"
CONFIG_DIR = Path.home() / ".remotepc"
TOKENS_FILE = CONFIG_DIR / "tokens.json"


# ---------------------------------------------------------------- input --
class InputController:
    """Bọc pynput; nạp chậm để server vẫn chạy được nếu không có màn hình."""

    def __init__(self):
        from pynput.keyboard import Controller as KC, Key
        from pynput.mouse import Controller as MC, Button
        self.kb, self.mouse, self.Key, self.Button = KC(), MC(), Key, Button
        self.wheel = [0, 0]
        K = Key
        self.keymap = {
            "enter": K.enter, "esc": K.esc, "tab": K.tab, "backspace": K.backspace,
            "delete": K.delete, "space": K.space, "up": K.up, "down": K.down,
            "left": K.left, "right": K.right, "home": K.home, "end": K.end,
            "pageup": K.page_up, "pagedown": K.page_down, "ctrl": K.ctrl,
            "alt": K.alt, "shift": K.shift, "win": K.cmd, "cmd": K.cmd,
            "capslock": K.caps_lock,
            **{f"f{i}": getattr(K, f"f{i}") for i in range(1, 13)},
        }
        for name, attr in [("volup", "media_volume_up"), ("voldown", "media_volume_down"),
                           ("mute", "media_volume_mute"), ("playpause", "media_play_pause"),
                           ("next", "media_next"), ("prev", "media_previous"),
                           ("printscreen", "print_screen")]:
            if hasattr(K, attr):
                self.keymap[name] = getattr(K, attr)

    def _key(self, name):
        name = name.lower()
        if name in self.keymap:
            return self.keymap[name]
        if len(name) == 1:
            return name
        raise ValueError(f"phím không hỗ trợ: {name}")

    def _button(self, b):
        return {"left": self.Button.left, "right": self.Button.right,
                "middle": self.Button.middle}.get(b, self.Button.left)

    def handle(self, msg, monitor):
        t = msg.get("t")
        if t == "move":  # di chuyển tương đối (touchpad), giữ trong màn hình đang xem
            x, y = self.mouse.position
            x = min(max(x + int(msg["dx"]), monitor["left"]), monitor["left"] + monitor["width"] - 1)
            y = min(max(y + int(msg["dy"]), monitor["top"]), monitor["top"] + monitor["height"] - 1)
            self.mouse.position = (int(x), int(y))
        elif t == "moveto":  # toạ độ chuẩn hoá 0..1 trên màn hình đang xem
            x = monitor["left"] + float(msg["x"]) * monitor["width"]
            y = monitor["top"] + float(msg["y"]) * monitor["height"]
            self.mouse.position = (int(x), int(y))
        elif t == "click":
            self.mouse.click(self._button(msg.get("b", "left")), int(msg.get("n", 1)))
        elif t == "down":
            self.mouse.press(self._button(msg.get("b", "left")))
        elif t == "up":
            self.mouse.release(self._button(msg.get("b", "left")))
        elif t == "scroll":
            self.mouse.scroll(int(msg.get("dx", 0)), int(msg.get("dy", 0)))
        elif t == "wheel":  # đơn vị 1/120 nấc; pynput chỉ lăn theo nấc nguyên
            self.wheel[0] += int(msg.get("dx", 0))
            self.wheel[1] += int(msg.get("dy", 0))
            nx, ny = int(self.wheel[0] / 120), int(self.wheel[1] / 120)
            if nx or ny:
                self.mouse.scroll(nx, ny)
                self.wheel[0] -= nx * 120
                self.wheel[1] -= ny * 120
        elif t == "kdown":
            self.kb.press(self._key(msg["k"]))
        elif t == "kup":
            self.kb.release(self._key(msg["k"]))
        elif t == "text":
            self.kb.type(str(msg["s"])[:2000])
        elif t == "key":
            self.kb.tap(self._key(msg["k"]))
        elif t == "combo":  # ví dụ ["ctrl","c"]
            keys = [self._key(k) for k in msg["k"]][:5]
            for k in keys:
                self.kb.press(k)
            for k in reversed(keys):
                self.kb.release(k)

    def ensure_on(self, m):
        """Đưa con trỏ vào giữa màn hình m nếu nó đang ở màn hình khác."""
        x, y = self.mouse.position
        if not (m["left"] <= x < m["left"] + m["width"] and m["top"] <= y < m["top"] + m["height"]):
            self.mouse.position = (m["left"] + m["width"] // 2, m["top"] + m["height"] // 2)

    def cursor(self):
        return self.mouse.position


# ---------------------------------------------------------------- auth --
class Auth:
    def __init__(self, reset=False):
        CONFIG_DIR.mkdir(exist_ok=True)
        self.tokens = set()
        if TOKENS_FILE.exists() and not reset:
            try:
                self.tokens = set(json.loads(TOKENS_FILE.read_text()))
            except Exception:
                pass
        self._save()
        self.pin = f"{secrets.randbelow(10**6):06d}"
        self.fails = {}  # ip -> (count, locked_until)

    def _save(self):
        TOKENS_FILE.write_text(json.dumps(sorted(self.tokens)))
        try:
            os.chmod(TOKENS_FILE, 0o600)
        except Exception:
            pass

    def pair(self, ip, pin):
        count, until = self.fails.get(ip, (0, 0))
        if time.time() < until:
            return None, f"Sai quá nhiều lần, thử lại sau {int(until - time.time())} giây"
        if not secrets.compare_digest(str(pin), self.pin):
            count += 1
            self.fails[ip] = (count, time.time() + 60 if count >= 5 else 0)
            return None, "Mã PIN không đúng"
        self.fails.pop(ip, None)
        tok = secrets.token_urlsafe(32)
        self.tokens.add(tok)
        self._save()
        return tok, None

    def ok(self, token):
        return bool(token) and any(secrets.compare_digest(token, t) for t in self.tokens)


# --------------------------------------------------------------- server --
def lan_ips():
    ips = set()
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ips.add(s.getsockname()[0])
        s.close()
    except Exception:
        pass
    try:
        for info in socket.getaddrinfo(socket.gethostname(), None, socket.AF_INET):
            ips.add(info[4][0])
    except Exception:
        pass
    return sorted(ip for ip in ips if not ip.startswith("127.")) or ["127.0.0.1"]


class Grabber:
    """mss không an toàn giữa các luồng: mỗi kết nối dùng 1 luồng riêng."""

    def __init__(self):
        from concurrent.futures import ThreadPoolExecutor
        self.pool = ThreadPoolExecutor(max_workers=1)
        self.sct = None

    def _sct(self):
        if self.sct is None:
            self.sct = (getattr(mss, "MSS", None) or mss.mss)()
        return self.sct

    def monitors(self):
        return self.pool.submit(lambda: list(self._sct().monitors)).result()

    async def jpeg(self, monitor, max_w, quality):
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            self.pool, lambda: grab_jpeg(self._sct(), monitor, max_w, quality))

    def close(self):
        def _c():
            if self.sct:
                self.sct.close()
        self.pool.submit(_c)
        self.pool.shutdown(wait=False)


def grab_jpeg(sct, monitor, max_w, quality):
    shot = sct.grab(monitor)
    img = Image.frombytes("RGB", shot.size, shot.bgra, "raw", "BGRX")
    if img.width > max_w:
        img = img.resize((max_w, round(img.height * max_w / img.width)), Image.BILINEAR)
    buf = io.BytesIO()
    img.save(buf, "JPEG", quality=quality, optimize=False)
    return buf.getvalue()


async def ws_handler(request):
    app = request.app
    if not app["auth"].ok(request.query.get("token", "")):
        raise web.HTTPUnauthorized(text="token không hợp lệ")
    ws = web.WebSocketResponse(heartbeat=20, max_msg_size=1 << 20)
    await ws.prepare(request)
    inp = app["input"]
    loop = asyncio.get_running_loop()
    state = {"mon": 1, "max_w": 1280, "q": 60, "fps": 15, "stream": True}
    ack = asyncio.Event()
    ack.set()

    grabber = Grabber()
    monitors = grabber.monitors()  # [0] = tất cả, [1..] = từng màn hình

    def mon():
        return monitors[state["mon"]] if state["mon"] < len(monitors) else monitors[1]

    await ws.send_json({"t": "hello", "host": socket.gethostname(),
                        "os": platform.system(), "input": inp is not None,
                        "version": "0.3.0", "monitors": [{"i": i, "w": m["width"], "h": m["height"]}
                                     for i, m in enumerate(monitors) if i > 0]})

    async def streamer():
        last = b""
        while not ws.closed:
            await ack.wait()
            t0 = time.monotonic()
            if state["stream"]:
                m = mon()
                jpg = await grabber.jpeg(m, state["max_w"], state["q"])
                cur = inp.cursor() if inp else None
                meta = {"t": "frame", "mon": state["mon"], "w": m["width"], "h": m["height"]}
                if cur:
                    meta["cx"] = (cur[0] - m["left"]) / m["width"]
                    meta["cy"] = (cur[1] - m["top"]) / m["height"]
                if jpg != last:
                    ack.clear()
                    await ws.send_json(meta)
                    await ws.send_bytes(jpg)
                    last = jpg
                else:
                    meta["same"] = True
                    await ws.send_json(meta)  # chỉ cập nhật con trỏ
            await asyncio.sleep(max(0, 1 / state["fps"] - (time.monotonic() - t0)))

    task = asyncio.create_task(streamer())
    try:
        async for msg in ws:
            if msg.type != WSMsgType.TEXT:
                continue
            try:
                data = json.loads(msg.data)
            except ValueError:
                continue
            t = data.get("t")
            if t == "ack":
                ack.set()
            elif t == "cfg":
                for k, lo, hi in (("mon", 0, 16), ("max_w", 320, 3840), ("q", 20, 95), ("fps", 1, 30)):
                    if k in data:
                        state[k] = max(lo, min(hi, int(data[k])))
                if "stream" in data:
                    state["stream"] = bool(data["stream"])
                if "mon" in data and inp is not None:
                    await loop.run_in_executor(None, inp.ensure_on, mon())
                ack.set()
            elif inp is not None:
                try:
                    await loop.run_in_executor(None, inp.handle, data, mon())
                except Exception as e:
                    await ws.send_json({"t": "err", "msg": str(e)})
    finally:
        task.cancel()
        grabber.close()
    return ws


async def pair_handler(request):
    try:
        body = await request.json()
    except Exception:
        body = {}
    tok, err = request.app["auth"].pair(request.remote, body.get("pin", ""))
    if err:
        return web.json_response({"error": err}, status=403)
    print(f"[+] Đã ghép nối thiết bị mới từ {request.remote}")
    return web.json_response({"token": tok})


async def check_handler(request):
    ok = request.app["auth"].ok(request.query.get("token", ""))
    return web.json_response({"ok": ok}, status=200 if ok else 401)


@web.middleware
async def no_cache(request, handler):
    resp = await handler(request)
    resp.headers["Cache-Control"] = "no-store"
    return resp


async def index(request):
    return web.FileResponse(WEB_DIR / "index.html")


def main():
    ap = argparse.ArgumentParser(description="RemotePC agent")
    ap.add_argument("--port", type=int, default=8765)
    ap.add_argument("--reset", action="store_true", help="xoá các thiết bị đã ghép nối")
    args = ap.parse_args()

    try:
        inp = InputController()
    except Exception as e:
        print(f"[!] Không điều khiển được chuột/bàn phím ({e}). Chỉ xem màn hình.")
        inp = None

    app = web.Application(middlewares=[no_cache])
    app["auth"] = Auth(reset=args.reset)
    app["input"] = inp
    app.router.add_get("/", index)
    app.router.add_post("/api/pair", pair_handler)
    app.router.add_get("/api/check", check_handler)
    app.router.add_get("/ws", ws_handler)
    app.router.add_static("/", WEB_DIR)

    pin = app["auth"].pin
    urls = [f"http://{ip}:{args.port}" for ip in lan_ips()]
    print("=" * 56)
    print("  RemotePC đang chạy. Trên điện thoại (cùng Wi-Fi) mở:")
    for u in urls:
        print(f"     {u}")
    print(f"  Mã PIN ghép nối: {pin}")
    print("=" * 56)
    try:
        import qrcode
        qr = qrcode.QRCode(border=1)
        qr.add_data(f"{urls[0]}/?pin={pin}")
        qr.print_ascii(invert=True)
        print("  Quét mã QR ở trên bằng camera điện thoại để ghép nối nhanh.")
    except Exception:
        pass
    web.run_app(app, host="0.0.0.0", port=args.port, print=None)


if __name__ == "__main__":
    sys.exit(main())
