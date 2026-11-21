import _winapi
import ctypes
import ctypes.wintypes
import http.server
import json
import os
import re
import shutil
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import time

CORE = os.path.abspath(sys.argv[1])
APP = os.path.abspath(sys.argv[2]) if len(sys.argv) > 2 else ""
SHOTS = os.path.abspath(sys.argv[3]) if len(sys.argv) > 3 else ""
TRAY = os.path.abspath(sys.argv[4]) if len(sys.argv) > 4 else ""
WORK = tempfile.mkdtemp(prefix="spofy-stand-")
USER = "00000000-0000-4000-8000-000000000001"
SITE = "http://cp.cloudflare.com/generate_204"
BARE = "http://1.1.1.1/"
KILLS = int(os.environ.get("SPOFY_KILLS") or 12)
SWITCHES = int(os.environ.get("SPOFY_SWITCHES") or 10)
failed = []


def expect(name, ok, detail=""):
    print("%s %s%s" % ("ok  " if ok else "FAIL", name, " " + detail if detail else ""), flush=True)
    if not ok:
        failed.append(name)


def ps(command):
    r = subprocess.run(["powershell", "-NoProfile", "-NonInteractive", "-Command", command], capture_output=True, text=True, timeout=90)
    return r.stdout.strip()


def fetch(limit=15):
    r = subprocess.run(["curl.exe", "-s", "-m", str(limit), "-o", "NUL", "-w", "%{http_code}", SITE], capture_output=True, text=True)
    return r.stdout.strip()


def attempt(limit):
    r = subprocess.run(["curl.exe", "-s", "-m", str(limit), "-o", "NUL", "-w", "%{http_code} %{time_namelookup} %{time_connect}", SITE], capture_output=True, text=True)
    return [r.returncode] + r.stdout.split()


def past(source, limit=4):
    r = subprocess.run(["curl.exe", "-s", "-m", str(limit), "--interface", source, "-o", "NUL", "-w", "%{http_code}", BARE], capture_output=True, text=True)
    return r.stdout.strip() != "000"


def until(what, limit=30.0, step=0.2):
    end = time.time() + limit
    while time.time() < end:
        v = what()
        if v:
            return v
        time.sleep(step)
    return None


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


class Lines:
    def __init__(self, name, args, **kw):
        self.name = name
        self.err = os.path.join(WORK, name + ".err")
        self.p = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(self.err, "w", encoding="utf-8"),
                                  text=True, encoding="utf-8", bufsize=1, **kw)
        self.seen = []
        self.lock = threading.Lock()
        threading.Thread(target=self.read, daemon=True).start()

    def read(self):
        for line in self.p.stdout:
            with self.lock:
                self.seen.append(line.strip())

    def lines(self):
        with self.lock:
            return list(self.seen)

    def event(self, kind, limit):
        def find():
            for line in self.lines():
                if line.startswith("{") and json.loads(line).get("event") == kind:
                    return json.loads(line)
        return until(find, limit, 0.1)

    def tail(self):
        return " | ".join(self.lines()[-6:] + open(self.err, encoding="utf-8", errors="replace").read().strip().splitlines()[-6:])[-1500:]


def server(port, uplink, outbound=None):
    config = {
        "log": {"loglevel": "info"},
        "inbounds": [{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": {"clients": [{"id": USER}], "decryption": "none"},
                      "streamSettings": {"network": "tcp", "security": "none"}}],
        "outbounds": [outbound or {"protocol": "freedom", "settings": {"domainStrategy": "UseIPv4"}, "streamSettings": {"sockopt": {"interface": uplink}}}],
    }
    s = Lines("server", [CORE, "engine"])
    s.p.stdin.write(json.dumps({"id": 1, "op": "start", "config": config}) + "\n")
    s.p.stdin.flush()
    started = until(lambda: [m for m in map(json.loads, [line for line in s.lines() if line.startswith("{")]) if m.get("re") == 1], 30, 0.1)
    if not started or not started[0].get("ok"):
        raise SystemExit("the test server did not start: %s" % s.tail())
    return s


def listed(port, *more):
    entries = [entry_for(p, "\U0001F3F3 (LOCAL %d)" % (n + 1) if more else "\U0001F3F3 (LOCAL)") for n, p in enumerate((port,) + more)]
    path = os.path.join(WORK, "sub%d.json" % len(entries))
    with open(path, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False)
    return path


def entry_for(port, remarks):
    return {
        "remarks": remarks,
        "dns": {"servers": ["8.8.8.8"], "queryStrategy": "UseIPv4"},
        "inbounds": [{"tag": "socks", "port": 10808, "listen": "127.0.0.1", "protocol": "socks", "settings": {"udp": True, "auth": "noauth"}}],
        "outbounds": [
            {"tag": "proxy", "protocol": "vless", "settings": {"vnext": [{"address": "127.0.0.1", "port": port, "users": [{"id": USER, "encryption": "none"}]}]},
             "streamSettings": {"network": "tcp", "security": "none"}},
            {"tag": "direct", "protocol": "freedom"},
            {"tag": "block", "protocol": "blackhole"},
            {"tag": "dns-out", "protocol": "dns"},
        ],
        "routing": {"domainStrategy": "AsIs", "rules": [{"type": "field", "port": "53", "outboundTag": "dns-out"}]},
    }


def adapter():
    return ps("(Get-NetAdapter -Name spofy0 -ErrorAction SilentlyContinue).Status")


class Entry(ctypes.Structure):
    _fields_ = [("size", ctypes.wintypes.DWORD), ("usage", ctypes.wintypes.DWORD), ("pid", ctypes.wintypes.DWORD), ("heap", ctypes.c_size_t),
                ("module", ctypes.wintypes.DWORD), ("threads", ctypes.wintypes.DWORD), ("parent", ctypes.wintypes.DWORD), ("priority", ctypes.c_long),
                ("flags", ctypes.wintypes.DWORD), ("exe", ctypes.c_wchar * 260)]


def engine_of(pid):
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.CreateToolhelp32Snapshot.restype = ctypes.wintypes.HANDLE
    kernel.Process32FirstW.argtypes = kernel.Process32NextW.argtypes = [ctypes.wintypes.HANDLE, ctypes.POINTER(Entry)]
    kernel.CloseHandle.argtypes = [ctypes.wintypes.HANDLE]
    snapshot = kernel.CreateToolhelp32Snapshot(2, 0)
    entry = Entry()
    entry.size = ctypes.sizeof(Entry)
    found = []
    more = kernel.Process32FirstW(snapshot, ctypes.byref(entry))
    while more:
        if entry.parent == pid and entry.exe.lower() == os.path.basename(CORE).lower():
            found.append(entry.pid)
        more = kernel.Process32NextW(snapshot, ctypes.byref(entry))
    kernel.CloseHandle(snapshot)
    return found


def run(sub, *flags):
    return Lines("run", [CORE, "run", "-sub", sub, "-server", "auto", "-log", "info", *flags], creationflags=subprocess.CREATE_NEW_PROCESS_GROUP)


def stop(core):
    core.p.send_signal(signal.CTRL_BREAK_EVENT)
    stopped = core.event("stopped", 30)
    gone = until(lambda: adapter() == "", 15, 0.5)
    return bool(stopped) and bool(gone)


def count(core, kind):
    return len([line for line in core.lines() if line.startswith("{") and json.loads(line).get("event") == kind])


def tls():
    openssl = shutil.which("openssl") or r"C:\Program Files\OpenSSL\bin\openssl.exe"
    conf = {
        "ca.cnf": "[req]\ndistinguished_name = dn\nprompt = no\nx509_extensions = ext\n[dn]\nCN = Spofy Stand CA\n[ext]\nbasicConstraints = critical,CA:TRUE\nkeyUsage = critical,keyCertSign,cRLSign\n",
        "srv.cnf": "[req]\ndistinguished_name = dn\nprompt = no\n[dn]\nCN = 127.0.0.1\n[ext]\nsubjectAltName = IP:127.0.0.1\nextendedKeyUsage = serverAuth\nbasicConstraints = CA:FALSE\nkeyUsage = critical,digitalSignature,keyEncipherment\n",
    }
    for name, text in conf.items():
        with open(os.path.join(WORK, name), "w") as f:
            f.write(text)
    for args in (["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", "ca.key", "-out", "ca.pem", "-days", "2", "-config", "ca.cnf"],
                 ["req", "-new", "-newkey", "rsa:2048", "-nodes", "-keyout", "srv.key", "-out", "srv.csr", "-config", "srv.cnf"],
                 ["x509", "-req", "-in", "srv.csr", "-CA", "ca.pem", "-CAkey", "ca.key", "-CAcreateserial", "-out", "srv.pem", "-days", "2", "-extfile", "srv.cnf", "-extensions", "ext"]):
        r = subprocess.run([openssl, *args], cwd=WORK, capture_output=True, text=True)
        if r.returncode != 0:
            raise SystemExit("openssl %s: %s" % (args[0], r.stderr[-400:]))
    r = subprocess.run(["certutil", "-addstore", "-f", "Root", os.path.join(WORK, "ca.pem")], capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit("certutil: %s" % r.stdout[-400:])


def page(body):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Profile-Title", "Spofy Stand")
            self.send_header("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(os.path.join(WORK, "srv.pem"), os.path.join(WORK, "srv.key"))
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, "https://127.0.0.1:%d/sub/stand" % srv.server_address[1]


class Pipe:
    def __init__(self, path):
        self.h = _winapi.CreateFile(path, 0x120089 | 0x2, 0, _winapi.NULL, _winapi.OPEN_EXISTING, _winapi.FILE_FLAG_OVERLAPPED, _winapi.NULL)
        self.seen, self.lock, self.n = [], threading.Lock(), 0
        threading.Thread(target=self.read, daemon=True).start()

    def read(self):
        rest = b""
        while True:
            try:
                ov, _ = _winapi.ReadFile(self.h, 65536, overlapped=True)
                ov.GetOverlappedResult(True)
            except OSError:
                return
            data = ov.getbuffer()
            if not data:
                return
            rest += bytes(data)
            *whole, rest = rest.split(b"\n")
            with self.lock:
                self.seen.extend(json.loads(line) for line in whole if line.strip())

    def ask(self, op, limit=30, **fields):
        self.n += 1
        n = self.n
        ov, _ = _winapi.WriteFile(self.h, (json.dumps({"id": n, "op": op, **fields}) + "\n").encode(), overlapped=True)
        ov.GetOverlappedResult(True)
        return self.wait(lambda m: m.get("re") == n, limit)

    def wait(self, want, limit):
        def find():
            with self.lock:
                return next((m for m in self.seen if want(m)), None)
        return until(find, limit, 0.05)

    def state(self):
        with self.lock:
            for m in reversed(self.seen):
                if m.get("ev") == "connection":
                    return m["connection"].get("state")
        return None

    def close(self):
        _winapi.CloseHandle(self.h)


PIPE = r"\\.\pipe\spofy"
JOURNAL = os.path.join(os.environ.get("ProgramData", "C:\\ProgramData"), "Spofy", "service.log")
journal = [""]


def keep_journal():
    if os.path.exists(JOURNAL):
        journal[0] = open(JOURNAL, encoding="utf-8", errors="replace").read()[-2000:]


OUTSIDER = r"""
import _winapi, sys, time
for _ in range(100):
    try:
        h = _winapi.CreateFile(sys.argv[1], 0x120089 | 0x2, 0, _winapi.NULL, _winapi.OPEN_EXISTING, 0, _winapi.NULL)
        break
    except OSError as e:
        if e.winerror == 5:
            sys.exit(4)
        if e.winerror != 231:
            sys.exit(5)
        time.sleep(0.05)
else:
    sys.exit(5)
try:
    _winapi.WriteFile(h, b'{"id":1,"op":"hello"}\n')
    sys.exit(0 if _winapi.ReadFile(h, 65536)[0] else 3)
except OSError:
    sys.exit(3)
"""


class StartupInfo(ctypes.Structure):
    _fields_ = [("cb", ctypes.c_uint32), ("reserved", ctypes.c_wchar_p), ("desktop", ctypes.c_wchar_p), ("title", ctypes.c_wchar_p)] + \
               [(n, ctypes.c_uint32) for n in ("x", "y", "width", "height", "columns", "rows", "fill", "flags")] + \
               [("show", ctypes.c_uint16), ("extra", ctypes.c_uint16), ("more", ctypes.c_void_p), ("stdin", ctypes.c_void_p), ("stdout", ctypes.c_void_p), ("stderr", ctypes.c_void_p)]


class ProcessInfo(ctypes.Structure):
    _fields_ = [("process", ctypes.c_void_p), ("thread", ctypes.c_void_p), ("pid", ctypes.c_uint32), ("tid", ctypes.c_uint32)]


class SidAttributes(ctypes.Structure):
    _fields_ = [("sid", ctypes.c_void_p), ("attributes", ctypes.c_uint32)]


def as_person(argv, limit):
    k, a = ctypes.WinDLL("kernel32", use_last_error=True), ctypes.WinDLL("advapi32", use_last_error=True)
    k.GetCurrentProcess.restype = ctypes.c_void_p
    own, person = ctypes.c_void_p(), ctypes.c_void_p()
    if not a.OpenProcessToken(ctypes.c_void_p(k.GetCurrentProcess()), 0x2 | 0x8 | 0x1, ctypes.byref(own)):
        raise OSError(ctypes.get_last_error())
    sid, size = ctypes.create_string_buffer(68), ctypes.c_uint32(68)
    a.CreateWellKnownSid(26, None, sid, ctypes.byref(size))
    drop = SidAttributes(ctypes.cast(sid, ctypes.c_void_p), 0)
    if not a.CreateRestrictedToken(own, 1, 1, ctypes.byref(drop), 0, None, 0, None, ctypes.byref(person)):
        raise OSError(ctypes.get_last_error())
    info, started = StartupInfo(), ProcessInfo()
    info.cb = ctypes.sizeof(info)
    line = ctypes.create_unicode_buffer(subprocess.list2cmdline(argv))
    try:
        if not a.CreateProcessAsUserW(person, None, line, None, None, False, 0x08000000, None, None, ctypes.byref(info), ctypes.byref(started)):
            raise OSError(ctypes.get_last_error())
        k.WaitForSingleObject(ctypes.c_void_p(started.process), int(limit * 1000))
        code = ctypes.c_uint32()
        k.GetExitCodeProcess(ctypes.c_void_p(started.process), ctypes.byref(code))
        k.CloseHandle(ctypes.c_void_p(started.process))
        k.CloseHandle(ctypes.c_void_p(started.thread))
        return code.value
    finally:
        k.CloseHandle(person)
        k.CloseHandle(own)


def system_owned(handle):
    owner, sd = ctypes.c_void_p(), ctypes.c_void_p()
    if ctypes.windll.advapi32.GetSecurityInfo(ctypes.c_void_p(handle), 6, 1, ctypes.byref(owner), None, None, None, ctypes.byref(sd)) != 0:
        return False
    ok = bool(ctypes.windll.advapi32.IsWellKnownSid(owner, 22))
    ctypes.windll.kernel32.LocalFree(sd)
    return ok


def pipe_open():
    return "spofy" in os.listdir("\\\\.\\pipe\\")


def service_pid():
    out = subprocess.run(["sc", "queryex", "Spofy"], capture_output=True, text=True).stdout
    for line in out.splitlines():
        if line.strip().startswith("PID"):
            return int(line.split(":")[1])
    return 0


def window():
    return ctypes.windll.user32.FindWindowW("SPOFY_WINDOW", None)


def shot(hwnd, name):
    box = ctypes.wintypes.RECT()
    ctypes.windll.user32.SetForegroundWindow(hwnd)
    time.sleep(1)
    ctypes.windll.user32.GetWindowRect(hwnd, ctypes.byref(box))
    path = os.path.join(SHOTS, name)
    ps("Add-Type -AssemblyName System.Drawing; $b = New-Object System.Drawing.Bitmap %d, %d; [System.Drawing.Graphics]::FromImage($b).CopyFromScreen(%d, %d, 0, 0, $b.Size); $b.Save('%s')"
       % (box.right - box.left, box.bottom - box.top, box.left, box.top, path))
    return os.path.exists(path), "%dx%d" % (box.right - box.left, box.bottom - box.top)


def window_scenario(source):
    app = subprocess.Popen([APP])
    hwnd = until(window, 30, 0.2)
    time.sleep(8)
    alive = app.poll() is None
    taken, size = shot(hwnd, "windows-home.png") if hwnd else (False, "")
    expect("the window opens and stays up next to the service", bool(hwnd) and alive and taken, "size=%s code=%s" % (size, app.poll()))
    if not hwnd:
        return
    again = subprocess.run([APP], timeout=30)
    expect("a second start brings the open window forward and exits", again.returncode == 0 and app.poll() is None and window() == hwnd, "code=%s" % again.returncode)
    ctypes.windll.user32.PostMessageW(hwnd, 0x10, 0, 0)
    closed = until(lambda: app.poll() is not None, 15, 0.2)
    expect("the window is closed: the VPN stays with the service", bool(closed) and fetch() == "204" and not past(source), "closed=%s" % bool(closed))
    if app.poll() is None:
        app.kill()


class Ident(ctypes.Structure):
    _fields_ = [("size", ctypes.c_uint32), ("hwnd", ctypes.c_void_p), ("id", ctypes.c_uint32), ("guid", ctypes.c_byte * 16)]


class Counters(ctypes.Structure):
    _fields_ = [("cb", ctypes.c_uint32), ("faults", ctypes.c_uint32)] + [(n, ctypes.c_size_t) for n in
                ("peak", "working", "peak_paged", "paged", "peak_nonpaged", "nonpaged", "commit", "peak_commit", "private")]


def tray_window():
    return ctypes.windll.user32.FindWindowW("SPOFY_TRAY", None)


def tray_icon(hwnd):
    rect = ctypes.windll.shell32.Shell_NotifyIconGetRect
    rect.restype = ctypes.c_long
    return rect(ctypes.byref(Ident(ctypes.sizeof(Ident), hwnd, 1)), ctypes.byref(ctypes.wintypes.RECT())) >= 0


def tray_post(hwnd, message, w=0, l=0):
    ctypes.windll.user32.PostMessageW(ctypes.c_void_p(hwnd), message, w, l)


def memory(pid):
    k = ctypes.windll.kernel32
    k.OpenProcess.restype = ctypes.c_void_p
    h = k.OpenProcess(0x1000, False, pid)
    c = Counters()
    c.cb = ctypes.sizeof(c)
    k.K32GetProcessMemoryInfo(ctypes.c_void_p(h), ctypes.byref(c), c.cb)
    k.CloseHandle(ctypes.c_void_p(h))
    return c.working / 2**20, c.private / 2**20


user32 = ctypes.WinDLL("user32", use_last_error=True)
EnumProc = ctypes.WINFUNCTYPE(ctypes.c_bool, ctypes.c_void_p, ctypes.c_void_p)
for name, args, result in [
    ("EnumWindows", [EnumProc, ctypes.c_void_p], ctypes.c_bool),
    ("GetClassNameW", [ctypes.c_void_p, ctypes.c_wchar_p, ctypes.c_int], ctypes.c_int),
    ("GetWindowThreadProcessId", [ctypes.c_void_p, ctypes.POINTER(ctypes.c_uint32)], ctypes.c_uint32),
    ("IsWindowVisible", [ctypes.c_void_p], ctypes.c_bool),
    ("SendMessageTimeoutW", [ctypes.c_void_p, ctypes.c_uint, ctypes.c_size_t, ctypes.c_ssize_t, ctypes.c_uint, ctypes.c_uint, ctypes.POINTER(ctypes.c_size_t)], ctypes.c_ssize_t),
    ("GetMenuItemCount", [ctypes.c_void_p], ctypes.c_int),
    ("GetMenuStringW", [ctypes.c_void_p, ctypes.c_uint, ctypes.c_wchar_p, ctypes.c_int, ctypes.c_uint], ctypes.c_int),
    ("GetMenuState", [ctypes.c_void_p, ctypes.c_uint, ctypes.c_uint], ctypes.c_uint),
    ("PostMessageW", [ctypes.c_void_p, ctypes.c_uint, ctypes.c_size_t, ctypes.c_ssize_t], ctypes.c_bool),
]:
    getattr(user32, name).argtypes, getattr(user32, name).restype = args, result


def menu_of(pid):
    found = []

    def each(h, _):
        name, owner = ctypes.create_unicode_buffer(16), ctypes.c_uint32()
        user32.GetClassNameW(h, name, 16)
        user32.GetWindowThreadProcessId(h, ctypes.byref(owner))
        if name.value == "#32768" and owner.value == pid and user32.IsWindowVisible(h):
            found.append(h)
        return True
    user32.EnumWindows(EnumProc(each), None)
    return found[0] if found else None


def menu_items(menu):
    got = ctypes.c_size_t()
    if not user32.SendMessageTimeoutW(menu, 0x01E1, 0, 0, 0x2, 2000, ctypes.byref(got)) or not got.value:
        return None, []
    items = []
    for i in range(max(user32.GetMenuItemCount(got.value), 0)):
        text = ctypes.create_unicode_buffer(256)
        user32.GetMenuStringW(got.value, i, text, 256, 0x400)
        items.append((text.value, user32.GetMenuState(got.value, i, 0x400)))
    return got.value, items


def menu_close(pid, menu, key):
    user32.PostMessageW(menu, 0x100, key, 0)
    until(lambda: not menu_of(pid), 5, 0.05)


def tray_menu(tray, hwnd, label, limit):
    end, shown = time.time() + limit, []
    while time.time() < end:
        tray_post(hwnd, 0x8001, 0, 0x007B | 1 << 16)
        menu = until(lambda: menu_of(tray.pid), 5, 0.05)
        if not menu:
            continue
        handle, items = menu_items(menu)
        shown = [text for text, _ in items]
        at = next((i for i, (text, state) in enumerate(items) if text == label and not state & 0x3), None)
        if handle is None or at is None:
            menu_close(tray.pid, menu, 0x1B)
            time.sleep(0.2)
            continue
        lit = lambda: [bool(user32.GetMenuState(handle, i, 0x400) & 0x80) for i in range(len(items))]
        for _ in range(2 * len(items)):
            now = lit()
            if now[at]:
                menu_close(tray.pid, menu, 0x0D)
                return shown
            user32.PostMessageW(menu, 0x100, 0x28, 0)
            until(lambda: lit() != now, 2, 0.02)
        menu_close(tray.pid, menu, 0x1B)
    print("     the tray menu never offered %a: %a" % (label, shown))
    return None


def reached(ui, mark, state, limit):
    def find():
        with ui.lock:
            return any(m.get("ev") == "connection" and m["connection"].get("state") == state for m in ui.seen[mark:])
    return until(find, limit, 0.05)


def through_tray(ui, tray, hwnd, label, state, limit):
    mark = len(ui.seen)
    return bool(tray_menu(tray, hwnd, label, 30)) and reached(ui, mark, state, limit)


def tray_scenario(ui):
    shell = "explorer.exe" in subprocess.run(["tasklist", "/FI", "IMAGENAME eq explorer.exe", "/NH"], capture_output=True, text=True).stdout.lower()
    tray = subprocess.Popen([TRAY])
    hwnd = until(tray_window, 15, 0.1)
    shown = bool(hwnd) and bool(until(lambda: tray_icon(hwnd), 15, 0.2))
    expect("the tray starts and puts its icon next to the clock", shown, "shell=%s code=%s" % (shell, tray.poll()))
    if not hwnd:
        if tray.poll() is None:
            tray.kill()
        return None
    again = subprocess.run([TRAY], timeout=15)
    expect("a second tray exits at once and the first stays", again.returncode == 0 and tray.poll() is None and tray_window() == hwnd, "code=%s" % again.returncode)
    off = through_tray(ui, tray, hwnd, "Выключить VPN", "idle", 30)
    on = bool(off) and through_tray(ui, tray, hwnd, "Включить VPN", "connected", 60)
    expect("the tray menu turns the VPN off and on through the service", bool(off) and bool(on) and fetch() == "204", "off=%s on=%s" % (bool(off), bool(on)))
    tray_post(hwnd, 0x8001, 0, 0x0400 | 1 << 16)
    opened = until(window, 30, 0.2)
    expect("the tray opens the window", bool(opened))
    if opened:
        tray_post(opened, 0x10)
        until(lambda: not window(), 15, 0.2)
    working, private = memory(tray.pid)
    expect("the tray takes little memory", private < 16, "working=%.1f MB private=%.1f MB" % (working, private))
    if shell:
        subprocess.run(["taskkill", "/F", "/IM", "explorer.exe"], capture_output=True)
        gone = until(lambda: not tray_icon(hwnd), 10, 0.2)
        subprocess.Popen(["explorer.exe"])
        back = until(lambda: tray_icon(hwnd), 60, 0.5)
        expect("the shell restarts: the tray puts its icon back", bool(gone) and bool(back) and tray.poll() is None, "gone=%s back=%s" % (bool(gone), bool(back)))
    else:
        print("     no shell on this machine: the icon after a shell restart is not checked")
    mark = len(ui.seen)
    shown = tray_menu(tray, hwnd, "Выключить VPN и выйти", 30) or [""]
    quit_off = reached(ui, mark, "idle", 15)
    ended = until(lambda: tray.poll() is not None, 15, 0.1)
    expect("quit while the VPN works: the menu says it works, the VPN is off, the tray is gone, no icon is left behind",
           shown[0].startswith("VPN работает") and bool(quit_off) and bool(ended) and tray.returncode == 0 and not tray_icon(hwnd),
           "status=%a off=%s code=%s" % (shown[0], bool(quit_off), tray.poll()))
    if tray.poll() is None:
        tray.kill()
    mark = len(ui.seen)
    ui.ask("connect", 60)
    reached(ui, mark, "connected", 60)
    tray = subprocess.Popen([TRAY])
    hwnd = until(tray_window, 15, 0.1)
    return (tray, hwnd) if hwnd else None


def tray_after_restart(started):
    tray, hwnd = started
    ui = Pipe(PIPE)
    ui.ask("hello")
    off = through_tray(ui, tray, hwnd, "Выключить VPN", "idle", 30)
    on = bool(off) and through_tray(ui, tray, hwnd, "Включить VPN", "connected", 60)
    expect("the service is killed and back: the tray finds it again and controls it", bool(off) and bool(on) and tray_icon(hwnd), "off=%s on=%s" % (bool(off), bool(on)))
    tray_post(hwnd, 0x16, 1, 1)
    ended = until(lambda: tray.poll() is not None, 15, 0.1)
    expect("the session ends: the tray leaves with its icon", bool(ended) and tray.returncode == 0 and not tray_icon(hwnd), "code=%s" % tray.poll())
    if tray.poll() is None:
        tray.kill()
    ui.close()


def kill_service():
    leaks, watching = [], threading.Event()
    watching.set()

    def watch():
        while watching.is_set():
            if past(source_address[0], 2):
                leaks.append(time.time())
    watcher = threading.Thread(target=watch, daemon=True)
    watcher.start()
    old = service_pid()
    at = time.time()
    subprocess.run(["taskkill", "/F", "/PID", str(old)], capture_output=True)
    back = until(lambda: service_pid() not in (0, old) and pipe_open() and adapter() == "Up" and fetch(4) == "204", 90, 0.3)
    took = time.time() - at
    watching.clear()
    watcher.join(10)
    return (took if back else None), len(leaks)


source_address = [""]


def seen_on(servers):
    return [len([line for line in s.lines() if "cp.cloudflare.com" in line]) for s in servers]


def received():
    value = ps("(Get-NetAdapterStatistics -Name spofy0 -ErrorAction SilentlyContinue).ReceivedBytes")
    return int(value) if value.isdigit() else -1


def switch_scenario(ui, servers):
    hello = ui.ask("hello")
    ids = [s["id"] for s in ((hello or {}).get("snapshot") or {}).get("servers") or []]
    if len(ids) < 2:
        expect("the subscription of the stand has two servers", False, str(hello)[:300])
        return
    log, going = [], threading.Event()
    going.set()

    def load():
        while going.is_set():
            began = time.time()
            got = attempt(4)
            log.append((began, time.time(), got))
            time.sleep(0.05)
    loader = threading.Thread(target=load, daemon=True)
    loader.start()
    time.sleep(1.5)
    replies, recreated, before, counts = [], 0, received(), seen_on(servers)
    for n in range(SWITCHES):
        at = time.time()
        r = ui.ask("connect", 30, server=ids[(n + 1) % 2])
        replies.append(round(time.time() - at, 2) if r and r.get("ok") else None)
        time.sleep(2.5)
        now = received()
        recreated += now < before
        before = now
    time.sleep(1.5)
    going.clear()
    loader.join(10)
    done = [(b, e, g) for b, e, g in log if g[1:2] == ["204"]]
    broken = [g for _, _, g in log if g[1:2] != ["204"]]
    stuck = [g for g in broken if g[0] == 6 or (g[0] == 28 and g[2] == "0.000000")]
    gap = max([b2 - e1 for (_, e1, _), (b2, _, _) in zip(done, done[1:])] or [0])
    slowest = max([e - b for b, e, _ in done] or [0])
    typical = sorted(r for r in replies if r is not None)
    typical = typical[len(typical) // 2] if typical else None
    spread = [b - a for a, b in zip(counts, seen_on(servers))]
    expect("the server is switched %d times: the tunnel stays, no request waits on names, a switch takes under half a second, the longest pause is short" % SWITCHES,
           None not in replies and recreated == 0 and not stuck and typical < 0.5 and gap < 1.0 and min(spread) > 0,
           "median=%s slowest_switch=%s replies=%s recreated=%d requests=%d failed=%d stuck_on_names=%d longest_pause=%.1f s slowest_request=%.1f s through=%s codes=%s"
           % (typical, max(replies, key=lambda r: r or 0), replies if len(replies) <= 20 else [r for r in replies if r is None or r >= 0.5],
              recreated, len(log), len(broken), len(stuck), gap, slowest, spread, sorted(set(g[0] for g in broken))))


def service_scenario(source, servers, sub):
    source_address[0] = source
    tls()
    srv, address = page(open(sub, "rb").read())
    r = subprocess.run([CORE, "install"], capture_output=True, text=True)
    up = r.returncode == 0 and until(pipe_open, 30, 0.2)
    expect("the service is installed and opens its channel", bool(up), "pid=%d %s" % (service_pid(), r.stderr.strip()[-300:]))
    if not up:
        return
    ui = Pipe(PIPE)
    expect("the service's channel belongs to the system: a program of the person cannot pose as it", system_owned(ui.h))
    hello = ui.ask("hello")
    expect("the interface talks to the service with the rights of a person at the computer", bool(hello) and hello.get("ok") and "snapshot" in hello, str(hello)[:200])
    plain = subprocess.run([sys.executable, "-c", OUTSIDER, PIPE], timeout=30).returncode
    outsider = as_person([sys.executable, "-c", OUTSIDER, PIPE], 30)
    if outsider == 4:
        print("     the session of this machine has no person at the computer: an outside program of a person is not checked")
    else:
        expect("the service does not answer another program of a person, only Spofy's own", plain == 0 and outsider == 3, "administrator=%d person=%d" % (plain, outsider))
    added = ui.ask("subscribe", 60, url=address)
    expect("a subscription over https is added inside the service", bool(added) and added.get("ok"), str(added)[:300])
    ui.ask("set", key="autoConnect", value=True)
    at = time.time()
    asked = ui.ask("connect", 60)
    connected = ui.wait(lambda m: m.get("ev") == "connection" and m["connection"].get("state") == "connected", 60)
    took = time.time() - at
    seen = sum(seen_on(servers))
    code = fetch()
    through = sum(seen_on(servers)) - seen
    expect("connect from the interface: the tunnel is up and another program goes through it", bool(asked) and bool(connected) and code == "204" and through > 0,
           "s=%.1f code=%s seen=%d %s" % (took, code, through, "" if connected else str(asked)[:200]))
    expect("connected through the service, a request from the uplink address does not leave", not past(source))
    switch_scenario(ui, servers)
    if APP:
        window_scenario(source)
    started = tray_scenario(ui) if TRAY else None
    ui.close()

    took, leaked = kill_service()
    expect("the service is killed: the system starts it again and it reconnects by itself", took is not None, "s=%.1f leaked=%d" % (took or -1, leaked))
    if started:
        tray_after_restart(started)

    subprocess.run(["sc", "stop", "Spofy"], capture_output=True, text=True)
    stopped = until(lambda: "STOPPED" in subprocess.run(["sc", "query", "Spofy"], capture_output=True, text=True).stdout, 40, 0.5)
    expect("the service stops on request: the adapter and the block are gone", bool(stopped) and until(lambda: adapter() == "", 20, 0.5) and fetch() == "204" and past(source),
           "adapter=%s" % adapter())
    subprocess.run(["sc", "start", "Spofy"], capture_output=True, text=True)
    expect("started again it connects by itself", bool(until(lambda: pipe_open() and adapter() == "Up" and fetch(4) == "204", 60, 0.5)))

    ui = Pipe(PIPE)
    ui.ask("set", key="killSwitch", value=True)
    ui.close()
    took, leaked = kill_service()
    expect("with the kill switch the service is killed: not one request leaves past the tunnel", took is not None and leaked == 0, "s=%.1f leaked=%d" % (took or -1, leaked))
    ui = Pipe(PIPE)
    ui.ask("set", key="autoConnect", value=False)
    ui.close()
    took, leaked = kill_service()
    expect("with the kill switch and no auto connect the killed service comes back connected", took is not None and leaked == 0, "s=%.1f leaked=%d" % (took or -1, leaked))

    subprocess.run(["sc", "stop", "Spofy"], capture_output=True, text=True)
    stopped = until(lambda: "STOPPED" in subprocess.run(["sc", "query", "Spofy"], capture_output=True, text=True).stdout, 40, 0.5)
    closed = bool(stopped) and until(lambda: adapter() == "", 20, 0.5) is not None and not past(source) and fetch(4) != "204"
    expect("the service is stopped with the kill switch: the internet stays closed", closed, "adapter=%s" % adapter())
    subprocess.run(["sc", "start", "Spofy"], capture_output=True, text=True)
    expect("started again with the block left it connects by itself", bool(until(lambda: pipe_open() and adapter() == "Up" and fetch(4) == "204", 60, 0.5)))

    ui = Pipe(PIPE)
    off = ui.ask("disconnect", 30)
    expect("a disconnect with the kill switch opens the internet", bool(off) and off.get("ok") and bool(until(lambda: past(source, 3) and fetch(6) == "204", 20, 0.5)), str(off)[:200])
    on = ui.ask("connect", 60)
    ui.close()
    expect("connected again with the kill switch", bool(on) and on.get("ok") and not past(source), str(on)[:200])
    keep_journal()
    r = subprocess.run([CORE, "uninstall"], capture_output=True, text=True)
    gone = r.returncode == 0 and until(lambda: subprocess.run(["sc", "query", "Spofy"], capture_output=True).returncode != 0 and not pipe_open(), 30, 0.5)
    expect("the service is removed with its channel", bool(gone), r.stderr.strip()[-300:])
    expect("removed while the kill switch held the block: the internet works directly", bool(until(lambda: past(source, 3) and fetch(6) == "204", 20, 0.5)), "adapter=%s" % adapter())
    srv.shutdown()
    subprocess.run(["certutil", "-delstore", "Root", "Spofy Stand CA"], capture_output=True)


def scenario():
    uplink = ps("(Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort-Object -Property RouteMetric, ifMetric | Select-Object -First 1 | Get-NetAdapter).Name")
    source = ps("(Get-NetIPAddress -InterfaceAlias '%s' -AddressFamily IPv4 | Select-Object -First 1).IPAddress" % uplink)
    expect("the machine reaches the sites without the tunnel", fetch() == "204" and past(source), "uplink=%s" % uplink)
    port = free_port()
    remote = server(port, uplink)
    sub = listed(port)

    core = run(sub)
    started = core.event("started", 60)
    expect("the tunnel comes up", bool(started) and adapter() == "Up", "ms=%s adapter=%s %s" % ((started or {}).get("ms"), adapter(), "" if started else core.tail()))
    code = fetch()
    through = [line for line in remote.lines() if "cp.cloudflare.com" in line]
    expect("a request of another program goes through the tunnel to the server", code == "204" and bool(through), "code=%s seen=%d" % (code, len(through)))
    expect("without the guard a request from the uplink address leaves past the tunnel: the stand can see a leak", past(source))
    expect("after the stop the adapter is gone and the site is reached directly", stop(core) and fetch() == "204", "adapter=%s" % adapter())

    core = run(sub, "-guard", "-check", "2s")
    started = core.event("started", 60)
    seen = len([line for line in remote.lines() if "cp.cloudflare.com" in line])
    code = fetch()
    through = len([line for line in remote.lines() if "cp.cloudflare.com" in line]) - seen
    expect("with the guard the tunnel works and names are resolved inside it", bool(started) and code == "204" and through > 0,
           "code=%s seen=%d %s" % (code, through, "" if started else core.tail()))
    expect("with the guard a request from the uplink address does not leave", not past(source))

    leaks, tries, watching = [], [], threading.Event()
    watching.set()

    def watch():
        while watching.is_set():
            tries.append(1)
            if past(source, 2):
                leaks.append(time.time())
    watcher = threading.Thread(target=watch, daemon=True)
    watcher.start()
    killed, slowest, why, took, recreated, before, shown = 0, 0.0, "", [], 0, received(), 0
    for n in range(KILLS):
        victims = until(lambda: engine_of(core.p.pid), 10, 0.05) or []
        if len(victims) != 1:
            why = "engines=%s" % victims
            break
        at = time.time()
        subprocess.run(["taskkill", "/F", "/PID", str(victims[0])], capture_output=True)
        trail = []

        def back_up():
            if engine_of(core.p.pid) in ([], victims):
                return False
            got = attempt(1)
            trail.append([round(time.time() - at, 1)] + got)
            return got[1:2] == ["204"]
        if not until(back_up, 40, 0.1):
            why = "no tunnel after kill %d: %s index=%s" % (n + 1, trail[-12:], ps("(Get-NetAdapter -Name spofy0 -ErrorAction SilentlyContinue).ifIndex"))
            break
        if time.time() - at > 1.5 and shown < 10:
            shown += 1
            print("     kill %d: %s" % (n + 1, trail))
        slowest = max(slowest, time.time() - at)
        took.append(round(time.time() - at, 1))
        killed += 1
        now = received()
        recreated += now < before
        before = now
    back = until(lambda: fetch(6) == "204", 30, 0.5)
    watching.clear()
    watcher.join(10)
    expect("the engine is killed %d times in a row: nothing leaves past the tunnel and the tunnel is back" % KILLS,
           killed == KILLS and not leaks and bool(back) and adapter() == "Up" and recreated == 0,
           "killed=%d leaks=%d tries=%d slowest=%.1f s gave_up=%d recreated=%d %s %s" % (killed, len(leaks), len(tries), slowest, count(core, "failed"), recreated, why, "" if killed == KILLS else core.tail()))
    print("     seconds back after each kill: %s" % took)
    for line in core.lines():
        if line.startswith("{") and json.loads(line).get("event") in ("failed", "restored", "crashed", "connecting", "reconnecting"):
            event = json.loads(line)
            print("     %s %s" % (event.get("event"), {k: v for k, v in event.items() if k not in ("event", "pid", "stderr")}))
    errors = {}
    for line in open(core.err, encoding="utf-8", errors="replace"):
        if "[Error]" in line or "[Warning]" in line or "retrying" in line:
            key = re.sub(r"\d+", "N", line.strip())[-220:]
            errors[key] = errors.get(key, 0) + 1
    for key, n in sorted(errors.items(), key=lambda kv: -kv[1])[:25]:
        print("     %4d x %s" % (n, key))
    expect("a stop takes the guard down: both ways work again", stop(core) and fetch() == "204" and past(source), "adapter=%s" % adapter())

    core = run(sub, "-guard")
    started = core.event("started", 60)
    held = bool(started) and not past(source)
    engines = engine_of(core.p.pid)
    subprocess.run(["taskkill", "/F", "/PID", str(core.p.pid)], capture_output=True)
    killed = time.time()
    gone = {}

    def cleared():
        if "engine" not in gone and not engine_of(core.p.pid):
            gone["engine"] = round(time.time() - killed, 1)
        if "adapter" not in gone and adapter() == "":
            gone["adapter"] = round(time.time() - killed, 1)
        return len(gone) == 2
    free = until(lambda: past(source, 3) and fetch(6) == "204", 30, 0.5)
    left = until(cleared, 20, 0.5)
    expect("the supervisor is killed: the block and the adapter are gone with it and the internet works", held and bool(free) and bool(left),
           "held=%s free=%s gone_after_s=%s engines_before=%s engines_left=%s adapter=%s" % (held, bool(free), gone, engines, engine_of(core.p.pid), adapter()))

    try:
        second = free_port()
        service_scenario(source, [remote, server(second, uplink)], listed(port, second))
    finally:
        if subprocess.run(["sc", "query", "Spofy"], capture_output=True).returncode == 0:
            keep_journal()
            subprocess.run([CORE, "uninstall"], capture_output=True)
    if failed:
        print("--- service\n" + journal[0])
        print("--- run\n%s\n--- server\n%s" % (core.tail(), remote.tail()))
        print(ps("Get-NetAdapter | Format-Table -AutoSize Name, InterfaceDescription, Status; Get-NetRoute -AddressFamily IPv4 | Sort-Object RouteMetric | Select-Object -First 12 | Format-Table -AutoSize DestinationPrefix, NextHop, InterfaceAlias, RouteMetric"))
    for p in (core.p, remote.p):
        if p.poll() is None:
            p.kill()


if __name__ == "__main__":
    began = time.time()
    scenario()
    print("%s in %.0f s, %d failed" % ("WINDOWS OK" if not failed else "WINDOWS FAILED", time.time() - began, len(failed)))
    sys.exit(1 if failed else 0)
