import http.server, json, os, re, signal, socket, socketserver, ssl, subprocess, sys, threading, time

APK, CORE, OUT = sys.argv[1], os.path.abspath(sys.argv[2]), os.path.abspath(sys.argv[3])
RELEASE = sys.argv[4] if len(sys.argv) > 4 else ""
INSIDE = "127.0.0.2"
PACKAGE = "app.spofy"
DATA = "/data/user/0/" + PACKAGE
GATE = "10.0.2.2"
USER = "00000000-0000-4000-8000-000000000001"
SERVERS = {"a": 4443, "b": 4444}
failed = []
started = []
remote = {}
seen = []
os.makedirs(OUT, exist_ok=True)


def run(*cmd, limit=120, data=None):
    r = subprocess.run(cmd, capture_output=True, timeout=limit, input=data)
    return r.returncode, (r.stdout + r.stderr).decode(errors="replace").strip()


def adb(*args, **kw):
    return run("adb", *args, **kw)[1]


def shell(command, **kw):
    return adb("shell", command, **kw)


def expect(name, ok, detail=""):
    print("%s %s %s" % ("ok  " if ok else "FAIL", name, detail), flush=True)
    if not ok:
        failed.append(name)
    return ok


class Abort(Exception):
    pass


def need(name, ok, detail=""):
    if not expect(name, ok, detail):
        raise Abort()


def until(what, limit=30.0, step=0.5):
    end = time.time() + limit
    while time.time() < end:
        v = what()
        if v:
            return v
        time.sleep(step)
    return None


def address():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.connect(("192.0.2.1", 9))
    return s.getsockname()[0]


HOST = address()


def config(name, port):
    return {
        "remarks": "\U0001F3F3 (%s)" % name.upper(),
        "dns": {"servers": ["8.8.8.8"], "queryStrategy": "UseIPv4"},
        "inbounds": [{"tag": "socks", "port": 10808, "listen": "127.0.0.1", "protocol": "socks", "settings": {"udp": True, "auth": "noauth"}}],
        "outbounds": [
            {"tag": "proxy", "protocol": "vless", "settings": {"vnext": [{"address": GATE, "port": port, "users": [{"id": USER, "encryption": "none"}]}]},
             "streamSettings": {"network": "tcp", "security": "none"}},
            {"tag": "direct", "protocol": "freedom"},
            {"tag": "block", "protocol": "blackhole"},
            {"tag": "dns-out", "protocol": "dns"},
        ],
        "routing": {"domainStrategy": "AsIs", "rules": [
            {"type": "field", "ip": ["8.8.8.8"], "outboundTag": "proxy"},
            {"type": "field", "port": "53", "outboundTag": "dns-out"},
        ]},
    }


BODY = json.dumps([config(n, p) for n, p in SERVERS.items()], ensure_ascii=False).encode()


class Server(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def server_bind(self):
        socketserver.TCPServer.server_bind(self)
        self.server_name, self.server_port = "site", self.server_address[1]


class Site(http.server.BaseHTTPRequestHandler):
    def reply(self, code, body=b"", headers=()):
        self.send_response(code)
        for k, v in headers:
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/generate_204":
            self.reply(204)
        elif self.path == "/ip":
            seen.append(self.client_address[0])
            self.reply(200, b"site sees " + self.client_address[0].encode())
        elif self.path == "/sub/EMULATORTOKEN" and self.server.server_port == 8443:
            self.reply(200, BODY, [("Content-Type", "application/json"), ("Subscription-Userinfo", "upload=1; download=2; total=1000; expire=1798761600"),
                                   ("Profile-Update-Interval", "1"), ("Profile-Title", "Emulator")])
        else:
            self.reply(404)

    def log_message(self, *a):
        pass


def host_side():
    code, out = run("openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1", "-nodes", "-days", "1", "-keyout", OUT + "/key.pem",
                    "-out", OUT + "/cert.pem", "-subj", "/CN=" + GATE, "-addext", "subjectAltName=IP:" + GATE)
    if code != 0:
        raise SystemExit("openssl: " + out)
    plain = Server(("0.0.0.0", 8081), Site)
    secure = Server(("0.0.0.0", 8443), Site)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(OUT + "/cert.pem", OUT + "/key.pem")
    secure.socket = context.wrap_socket(secure.socket, server_side=True)
    for s in (plain, secure):
        threading.Thread(target=s.serve_forever, daemon=True).start()
    for name, port in SERVERS.items():
        config = {
            "log": {"loglevel": "info"},
            "inbounds": [{"tag": "in", "listen": "0.0.0.0", "port": port, "protocol": "vless", "settings": {"clients": [{"id": USER}], "decryption": "none"},
                          "streamSettings": {"network": "tcp", "security": "none"}}],
            "outbounds": [
                {"tag": "out", "protocol": "freedom", "settings": {"domainStrategy": "UseIPv4"}},
                {"tag": "site", "protocol": "freedom", "sendThrough": INSIDE, "settings": {"finalRules": [{"action": "allow", "ip": [HOST + "/32"]}]}},
            ],
            "routing": {"rules": [{"type": "field", "ip": [HOST + "/32"], "outboundTag": "site"}]},
        }
        log = open(OUT + "/server-%s.log" % name, "w", buffering=1)
        p = subprocess.Popen([CORE, "engine"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=log, text=True, bufsize=1)
        started.append(p)
        remote[name] = p
        p.stdin.write(json.dumps({"id": 1, "op": "start", "config": config}) + "\n")
        while True:
            line = p.stdout.readline()
            if not line:
                raise SystemExit("server %s did not start" % name)
            m = json.loads(line)
            if m.get("re") == 1:
                if not m.get("ok"):
                    raise SystemExit("server %s rejected its config: %s" % (name, line))
                break

        def drain(stream=p.stdout, out=log):
            for line in stream:
                out.write(line)
        threading.Thread(target=drain, daemon=True).start()


def accepted(name, what):
    return len([l for l in open(OUT + "/server-%s.log" % name).read().splitlines() if "accepted" in l and what in l])


def outside():
    return len([p for p in seen if p != INSIDE])


def through():
    return fetch(HOST, 8081, "/ip").endswith("site sees " + INSIDE)


def tunnel():
    m = re.search(r"\d+: (\S+)\s+inet 172\.19\.0\.1", shell("ip -o addr"))
    return m.group(1) if m else ""


def started_in():
    times = []
    for _ in range(3):
        shell("am force-stop %s" % PACKAGE)
        time.sleep(1)
        m = re.search(r"TotalTime: (\d+)", shell("am start -W -n %s/.MainActivity" % PACKAGE, limit=120))
        if m:
            times.append(int(m.group(1)))
        until(lambda: "core.sock" in shell("ls %s/files" % DATA), 30)
    return sorted(times)


class Channel:
    def __init__(self, port):
        self.s = socket.create_connection(("127.0.0.1", port), timeout=90)
        self.f = self.s.makefile("r")
        self.n = 0
        self.events = []

    def call(self, op, **fields):
        self.n += 1
        began = time.time()
        self.s.sendall((json.dumps({"id": self.n, "op": op, **fields}) + "\n").encode())
        while True:
            line = self.f.readline()
            if not line:
                raise Abort()
            m = json.loads(line)
            if m.get("re") == self.n:
                m["ms"] = int((time.time() - began) * 1000)
                return m
            self.events.append(m)

    def active(self):
        return self.call("hello")["snapshot"]["connection"]


def fetch(target, port, path, host=None):
    request = "GET %s HTTP/1.0\\r\\nHost: %s\\r\\n\\r\\n" % (path, host or target)
    return shell("printf '%s' | nc -w 8 %s %d" % (request, target, port), limit=30)


def processes():
    out = {}
    for line in shell("ps -A -o PID,RSS,ARGS").splitlines():
        parts = line.split(None, 2)
        if len(parts) == 3 and (PACKAGE in parts[2] or "libspofycore.so" in parts[2]) and parts[0].isdigit():
            out[int(parts[0])] = (int(parts[1]), parts[2])
    return out


def engines():
    return [pid for pid, (_, args) in processes().items() if args.rstrip().endswith("libspofycore.so engine")]


def memory():
    total = {}
    for pid, (_, args) in processes().items():
        text = shell("dumpsys meminfo %d" % pid, limit=60)
        m = re.search(r"TOTAL PSS:\s+(\d+)", text) or re.search(r"TOTAL\s+(\d+)", text)
        name = "interface" if args.strip() == PACKAGE else "service" if args.strip() == PACKAGE + ":core" else "engine" if args.rstrip().endswith(" engine") else "core"
        if m:
            total[name] = total.get(name, 0) + int(m.group(1)) // 1024
    return total


def screen(name):
    with open("%s/%s.png" % (OUT, name), "wb") as out:
        out.write(subprocess.run(["adb", "exec-out", "screencap", "-p"], capture_output=True, timeout=60).stdout)


def scenario():
    print("host address %s, apk %.1f MB" % (HOST, os.path.getsize(APK) / 1e6), flush=True)
    adb("wait-for-device")
    adb("root")
    time.sleep(3)
    adb("wait-for-device")
    code, out = run("adb", "install", "-r", "-g", APK, limit=300)
    need("the app installs", code == 0 and "Success" in out, out[-300:])
    shell("appops set %s ACTIVATE_VPN allow" % PACKAGE)
    cert = open(OUT + "/cert.pem", "rb").read()
    run("adb", "shell", "run-as %s sh -c 'mkdir -p files && cat > files/test-ca.pem'" % PACKAGE, data=cert)
    need("the test certificate is in the app folder", "test-ca.pem" in shell("run-as %s ls files" % PACKAGE))
    shell("am start -W -n %s/.MainActivity" % PACKAGE, limit=120)
    need("the app starts and its core listens", bool(until(lambda: "core.sock" in shell("ls %s/files" % DATA), 60)), shell("ls -la %s/files" % DATA))
    adb("forward", "tcp:9099", "localfilesystem:%s/files/core.sock" % DATA)
    ui = Channel(9099)
    hello = ui.call("hello")
    need("the core answers on its socket", hello.get("ok") and hello["snapshot"]["servers"] == [], str(hello)[:300])
    print("core version %s" % hello["snapshot"].get("version"), flush=True)
    time.sleep(2)
    screen("1-start")
    kinds = [args for _, args in processes().values()]
    expect("the service runs in its own process", any(a.strip() == PACKAGE + ":core" for a in kinds) and any(a.strip() == PACKAGE for a in kinds), str(kinds))

    s = ui.call("subscribe", url="https://example.com/sub/none")
    expect("the core resolves names and trusts the system certificates", not s["ok"] and s.get("code") in ("sub_status", "sub_format"), "code=%s %s" % (s.get("code"), s.get("error", "")[:200]))
    s = ui.call("subscribe", url="https://%s:8443/sub/EMULATORTOKEN" % GATE)
    need("subscribe", s["ok"], "ms=%d %s %s" % (s["ms"], s.get("code"), s.get("error", "")[:300]))
    m = ui.call("measure")
    servers = ([e for e in ui.events if e.get("ev") == "servers"] or [{"servers": []}])[-1]["servers"]
    expect("measure both servers", m["ok"] and len([x for x in servers if x["delayMs"] > 0]) == 2, "ms=%d delays=%s" % (m["ms"], {x["id"]: x["delayMs"] for x in servers}))
    direct = fetch(HOST, 8081, "/ip")
    expect("without the tunnel the site is reached directly", "site sees" in direct and not direct.endswith(INSIDE), direct[-40:])

    c = ui.call("connect", server="auto")
    first = ui.active()["server"]
    device = tunnel()
    need("connect", c["ok"] and first in SERVERS and device != "", "ms=%d server=%s %s %s" % (c["ms"], first, c.get("code"), c.get("error", "")[:300]))
    body = fetch(HOST, 8081, "/ip")
    need("another app's traffic goes through the tunnel", body.endswith("site sees " + INSIDE) and accepted(first, "%s:8081" % HOST) > 0, "body=%s accepted=%d" % (body[-60:], accepted(first, "%s:8081" % HOST)))
    leaked, flowing = outside(), len(seen)
    loop = "while [ -e /data/local/tmp/noise ]; do printf 'GET /ip HTTP/1.0\\r\\n\\r\\n' | nc -w 2 %s 8081 >/dev/null 2>&1; sleep 0.2; done\n" % HOST
    run("adb", "shell", "cat > /data/local/tmp/noise.sh", data=loop.encode())
    shell("touch /data/local/tmp/noise; nohup sh /data/local/tmp/noise.sh >/dev/null 2>&1 &")
    time.sleep(3)
    expect("background requests from another app are running", len(seen) > flowing + 3, "requests=%d" % (len(seen) - flowing))
    body = fetch("www.gstatic.com", 80, "/generate_204")
    expect("a site by name opens through the tunnel", " 204" in body, body[:80])
    t = ui.call("traffic")
    expect("traffic is counted", t["traffic"]["down"] > 0 and t["traffic"]["up"] > 0, str(t["traffic"]))
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("the service is in the foreground with a notification", "isForeground=true" in service, re.sub(r"\s+", " ", service)[:200])
    time.sleep(1)
    screen("2-connected")
    shell("cmd statusbar expand-notifications")
    time.sleep(1.5)
    screen("3-notification")
    shell("cmd statusbar collapse")
    print("memory while connected, MB: %s" % memory(), flush=True)
    rounds = []
    for _ in range(3):
        k = ui.call("check")
        rounds.append((k["ms"], ui.active()["delayMs"]))
    m = ui.call("measure")
    servers = ([e for e in ui.events if e.get("ev") == "servers"] or [{"servers": []}])[-1]["servers"]
    print("check through the tunnel (took ms, delay ms): %s; measured outside the tunnel while it is up: ms=%d %s" % (rounds, m["ms"], {x["id"]: x["delayMs"] for x in servers}), flush=True)

    other = [n for n in SERVERS if n != first][0]
    c = ui.call("connect", server=other)
    expect("another server on the same tunnel", c["ok"] and through() and accepted(other, "%s:8081" % HOST) > 0 and tunnel() == device, "ms=%d device=%s" % (c["ms"], tunnel()))
    kills = 0
    for _ in range(3):
        for pid in engines():
            shell("kill -9 %d" % pid)
            kills += 1
        time.sleep(1.5)
    back = until(through, 20)
    expect("engine killed three times: traffic is back on the same tunnel", kills == 3 and bool(back) and tunnel() == device, "kills=%d device=%s" % (kills, tunnel()))

    shell("input keyevent KEYCODE_HOME")
    time.sleep(1)
    shell("am kill %s" % PACKAGE)
    time.sleep(2)
    alive = [args.strip() for _, args in processes().values()]
    expect("the interface is closed: the tunnel keeps working", through() and PACKAGE + ":core" in alive, "processes=%s" % alive)
    shell("am start -W -n %s/.MainActivity" % PACKAGE, limit=120)
    time.sleep(4)
    screen("4-reopened")
    expect("the interface is opened again", PACKAGE in [args.strip() for _, args in processes().values()])

    c = ui.call("connect", server="auto")
    held = ui.active()["server"]
    remote[held].send_signal(signal.SIGSTOP)
    began = time.time()
    moved = until(lambda: ui.active()["state"] == "connected" and ui.active()["server"] != held and through(), 60, 0.2)
    remote[held].send_signal(signal.SIGCONT)
    expect("a server went silent under traffic: the app moved to the other one by itself on the same tunnel", c["ok"] and bool(moved) and tunnel() == device,
           "s=%.1f state=%s server=%s" % (time.time() - began, ui.active()["state"], ui.active()["server"]))
    shell("svc wifi disable")
    began = time.time()
    moved = until(lambda: ui.active()["state"] == "connected" and through(), 60)
    expect("Wi-Fi is off: traffic goes on over the mobile network on the same tunnel", bool(moved) and tunnel() == device, "s=%.1f state=%s device=%s" % (time.time() - began, ui.active()["state"], tunnel()))
    shell("svc wifi enable")
    time.sleep(6)
    again = until(lambda: ui.active()["state"] == "connected" and through(), 60)
    expect("Wi-Fi is back: still connected on the same tunnel", bool(again) and tunnel() == device, "state=%s" % ui.active()["state"])
    shell("cmd connectivity airplane-mode enable")
    began = time.time()
    off = until(lambda: ui.active()["state"] == "offline", 40, 0.1)
    expect("no network: the state says so and the tunnel stays", bool(off) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
    screen("5-offline")
    shell("cmd connectivity airplane-mode disable")
    began = time.time()
    on = until(lambda: ui.active()["state"] == "connected" and through(), 120)
    expect("the network is back: connected again without being asked", bool(on) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
    shell("rm -f /data/local/tmp/noise")
    time.sleep(3)
    expect("nothing reached the site past the tunnel while it was up", outside() == leaked, "requests=%d past=%d" % (len(seen), outside() - leaked))

    d = ui.call("disconnect")
    gone = until(lambda: tunnel() == "", 10)
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("disconnect removes the tunnel and the notification", d["ok"] and bool(gone) and "isForeground=true" not in service, "ms=%d" % d["ms"])
    time.sleep(1)
    screen("6-disconnected")
    print("memory when idle, MB: %s" % memory(), flush=True)
    state = shell("cat %s/files/state/state | head -c 4000" % DATA)
    expect("saved state has nothing in the clear", "EMULATORTOKEN" not in state and "outbounds" not in state and len(state) > 40, "bytes=%d" % len(state))
    key = shell("cat %s/files/key | wc -c" % DATA)
    expect("the key file holds only the wrapped key", key.strip() == "60", "bytes=%s" % key.strip())
    ui.s.close()

    shell("am force-stop %s" % PACKAGE)
    time.sleep(1)
    asked = shell("am start-foreground-service -a android.net.VpnService -n %s/.CoreService" % PACKAGE)
    began = time.time()
    up = until(lambda: tunnel() != "" and through(), 40, 0.2)
    alive = [args.strip() for _, args in processes().values()]
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("started by the system without the interface: the VPN comes up by itself", bool(up) and PACKAGE not in alive and "isForeground=true" in service,
           "s=%.1f processes=%d %s" % (time.time() - began, len(alive), asked[-120:]))
    later = Channel(9099)
    d = later.call("disconnect")
    gone = until(lambda: tunnel() == "" and "isForeground=true" not in shell("dumpsys activity services %s/.CoreService" % PACKAGE), 10)
    expect("disconnect after a start by the system removes the tunnel and the notification", d["ok"] and bool(gone))
    later.s.close()

    if RELEASE:
        adb("uninstall", PACKAGE, limit=120)
        code, out = run("adb", "install", "-g", RELEASE, limit=300)
        need("the release build installs", code == 0 and "Success" in out, out[-300:])
        times = started_in()
        time.sleep(3)
        screen("7-release")
        usage = memory()
        expect("the release build starts and its core listens", len(times) == 3 and "core.sock" in shell("ls %s/files" % DATA), "ms=%s" % times)
        print("release build: apk %.1f MB, cold start %s ms, memory %s MB" % (os.path.getsize(RELEASE) / 1e6, times, usage), flush=True)


began = time.time()
try:
    host_side()
    scenario()
except Abort:
    pass
finally:
    open(OUT + "/logcat.txt", "w").write(adb("logcat", "-d", "-v", "time", limit=120))
    if failed:
        print("--- logcat\n" + "\n".join([l for l in open(OUT + "/logcat.txt").read().splitlines() if "/spofy" in l or "AndroidRuntime" in l][-80:]))
        for name in SERVERS:
            print("--- server %s\n%s" % (name, open(OUT + "/server-%s.log" % name).read()[-800:]))
        screen("failed")
    for p in started:
        p.kill()
    for name in ("key.pem",):
        if os.path.exists(OUT + "/" + name):
            os.remove(OUT + "/" + name)
print("%s in %.0f s, %d failed" % ("ANDROID OK" if not failed else "ANDROID FAILED", time.time() - began, len(failed)))
sys.exit(1 if failed else 0)
