import ctypes
import ctypes.wintypes
import json
import os
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time

CORE = os.path.abspath(sys.argv[1])
WORK = tempfile.mkdtemp(prefix="spofy-stand-")
USER = "00000000-0000-4000-8000-000000000001"
SITE = "http://cp.cloudflare.com/generate_204"
BARE = "http://1.1.1.1/"
KILLS = int(os.environ.get("SPOFY_KILLS") or 12)
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


def server(port, uplink):
    config = {
        "log": {"loglevel": "info"},
        "inbounds": [{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": {"clients": [{"id": USER}], "decryption": "none"},
                      "streamSettings": {"network": "tcp", "security": "none"}}],
        "outbounds": [{"protocol": "freedom", "settings": {"domainStrategy": "UseIPv4"}, "streamSettings": {"sockopt": {"interface": uplink}}}],
    }
    s = Lines("server", [CORE, "engine"])
    s.p.stdin.write(json.dumps({"id": 1, "op": "start", "config": config}) + "\n")
    s.p.stdin.flush()
    started = until(lambda: [m for m in map(json.loads, [line for line in s.lines() if line.startswith("{")]) if m.get("re") == 1], 30, 0.1)
    if not started or not started[0].get("ok"):
        raise SystemExit("the test server did not start: %s" % s.tail())
    return s


def listed(port):
    path = os.path.join(WORK, "sub.json")
    entry = {
        "remarks": "\U0001F3F3 (LOCAL)",
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
    with open(path, "w", encoding="utf-8") as f:
        json.dump([entry], f, ensure_ascii=False)
    return path


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
    killed, slowest, why = 0, 0.0, ""
    for n in range(KILLS):
        victims = until(lambda: engine_of(core.p.pid), 10, 0.05) or []
        if len(victims) != 1:
            why = "engines=%s" % victims
            break
        at = time.time()
        subprocess.run(["taskkill", "/F", "/PID", str(victims[0])], capture_output=True)
        if not until(lambda: engine_of(core.p.pid) not in ([], victims) and fetch(4) == "204", 40, 0.1):
            why = "no tunnel after kill %d" % (n + 1)
            break
        slowest = max(slowest, time.time() - at)
        killed += 1
    back = until(lambda: fetch(6) == "204", 30, 0.5)
    watching.clear()
    watcher.join(10)
    expect("the engine is killed %d times in a row: nothing leaves past the tunnel and the tunnel is back" % KILLS,
           killed == KILLS and not leaks and bool(back) and adapter() == "Up", "killed=%d leaks=%d tries=%d slowest=%.1f s gave_up=%d %s %s" % (killed, len(leaks), len(tries), slowest, count(core, "failed"), why, "" if killed == KILLS else core.tail()))
    expect("a stop takes the guard down: both ways work again", stop(core) and fetch() == "204" and past(source), "adapter=%s" % adapter())

    core = run(sub, "-guard")
    started = core.event("started", 60)
    held = bool(started) and not past(source)
    subprocess.run(["taskkill", "/F", "/PID", str(core.p.pid)], capture_output=True)
    free = until(lambda: past(source, 3) and fetch(6) == "204", 30, 0.5)
    left = until(lambda: adapter() == "" and not engine_of(core.p.pid), 20, 0.5)
    expect("the supervisor is killed: the block and the adapter are gone with it and the internet works", held and bool(free) and bool(left),
           "held=%s free=%s adapter=%s" % (held, bool(free), adapter()))

    if failed:
        print("--- run\n%s\n--- server\n%s" % (core.tail(), remote.tail()))
        print(ps("Get-NetAdapter | Format-Table -AutoSize Name, InterfaceDescription, Status; Get-NetRoute -AddressFamily IPv4 | Sort-Object RouteMetric | Select-Object -First 12 | Format-Table -AutoSize DestinationPrefix, NextHop, InterfaceAlias, RouteMetric"))
    for p in (core.p, remote.p):
        if p.poll() is None:
            p.kill()


began = time.time()
scenario()
print("%s in %.0f s, %d failed" % ("WINDOWS OK" if not failed else "WINDOWS FAILED", time.time() - began, len(failed)))
sys.exit(1 if failed else 0)
