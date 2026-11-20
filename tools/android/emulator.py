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
asked = []
kicks = []
seen6 = []
panel = {"ended": False}
phone = {"sdk": 0, "nc": "nc", "limit": "timeout %d "}
os.makedirs(OUT, exist_ok=True)


def run(*cmd, limit=120, data=None):
    try:
        r = subprocess.run(cmd, capture_output=True, timeout=limit, input=data)
    except subprocess.TimeoutExpired:
        return 124, ""
    return r.returncode, (r.stdout + r.stderr).decode(errors="replace").strip()


def stamp():
    now = time.time()
    return time.strftime("%H:%M:%S", time.gmtime(now)) + ".%03d" % (now % 1 * 1000)


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
ENDED = json.dumps([{"remarks": line, "outbounds": [
    {"tag": "proxy", "protocol": "vless", "settings": {"vnext": [{"address": "0.0.0.0", "port": 1, "users": [{"id": USER, "encryption": "none"}]}]},
     "streamSettings": {"network": "tcp", "security": "none"}},
    {"tag": "direct", "protocol": "freedom"},
]} for line in ("\u23f0 Подписка закончилась", "VPN выключен до продления")], ensure_ascii=False).encode()


class Server(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def server_bind(self):
        socketserver.TCPServer.server_bind(self)
        self.server_name, self.server_port = "site", self.server_address[1]


class Server6(Server):
    address_family = socket.AF_INET6


class Site(http.server.BaseHTTPRequestHandler):
    def reply(self, code, body=b"", headers=()):
        self.send_response(code)
        for k, v in headers:
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.startswith("/sub/"):
            asked.append(self.path)
        if self.path == "/generate_204":
            self.reply(204)
        elif self.path == "/ip":
            (seen6 if self.server.address_family == socket.AF_INET6 else seen).append(self.client_address[0])
            self.reply(200, b"site sees " + self.client_address[0].encode())
        elif self.path == "/sub/EMULATORTOKEN" and self.server.server_port == 8443:
            self.reply(200, ENDED if panel["ended"] else BODY,
                       [("Content-Type", "application/json"), ("Subscription-Userinfo", "upload=1; download=2; total=1000; expire=%d" % (1700000000 if panel["ended"] else 1798761600)),
                        ("Profile-Update-Interval", "1"), ("Profile-Title", "Emulator"), ("Profile-Web-Page-Url", "https://%s:8443/page" % GATE)])
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
    try:
        threading.Thread(target=Server6(("::", 8083), Site).serve_forever, daemon=True).start()
    except OSError as e:
        print("the host has no IPv6: %s" % e, flush=True)
    counted = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    counted.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    counted.bind(("0.0.0.0", 8082))
    counted.listen(64)

    def count():
        while True:
            peer, _ = counted.accept()
            kicks.append(1)
            peer.close()
    threading.Thread(target=count, daemon=True).start()
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
                out.write(stamp() + " " + line)
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


def speed():
    took = []
    for _ in range(3):
        began = time.time()
        shell("i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done", limit=120)
        took.append(int((time.time() - began) * 1000))
    return min(took)


def installed():
    base = shell("pm path %s" % PACKAGE).splitlines()[0].split(":", 1)[-1].rsplit("/", 1)[0]
    return int(shell("du -sk %s" % base).split()[0]) / 1024


def started_in():
    times = []
    for _ in range(5):
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


def piped(seconds, target, port):
    return "%s%s -w %d %s %s" % (phone["limit"] % seconds if phone["limit"] else "", phone["nc"], seconds, target, port)


def learn():
    phone["sdk"] = int(shell("getprop ro.build.version.sdk").strip() or 0)
    if not shell("timeout 1 true; echo rc=$?").endswith("rc=0"):
        phone["limit"] = ""
    for tool in ("nc", "/system/xbin/nc", "toybox nc", "busybox nc"):
        phone["nc"] = tool
        if " 204" in fetch(HOST, 8081, "/generate_204"):
            break
    else:
        phone["nc"] = ""
    print("device: Android %s (API %d, %s), connections are made with %r, time limit %r"
          % (shell("getprop ro.build.version.release").strip(), phone["sdk"], shell("getprop ro.product.cpu.abi").strip(), phone["nc"], phone["limit"]), flush=True)
    need("the device can open connections for the checks", phone["nc"] != "")


def fetch(target, port, path, host=None):
    request = "GET %s HTTP/1.0\\r\\nHost: %s\\r\\n\\r\\n" % (path, host or target)
    return shell("printf '%s' | %s" % (request, piped(8, target, port)), limit=30)


def as_shell(path="/ip"):
    return shell("su 2000,2000,3003 sh /data/local/tmp/get.sh %s 8081 %s" % (HOST, path), limit=30)


def shown():
    shell("uiautomator dump /sdcard/ui.xml", limit=60)
    return " | ".join(re.findall(r'content-desc="([^"]+)"', shell("cat /sdcard/ui.xml", limit=30)))


def tap(*labels):
    for _ in range(6):
        shell("uiautomator dump /sdcard/ui.xml", limit=60)
        page = shell("cat /sdcard/ui.xml", limit=30)
        for label in labels:
            m = re.search(r'content-desc="%s[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"' % re.escape(label), page)
            if m:
                left, top, right, bottom = map(int, m.groups())
                shell("input tap %d %d" % ((left + right) // 2, (top + bottom) // 2))
                time.sleep(1.5)
                return label
        time.sleep(1)
    return ""


def linked(kind):
    return re.search(r"ni\{%s(\[[^\]]*\])? CONNECTED" % kind, shell("dumpsys connectivity")) is not None


def processes():
    listing = "for f in $(grep -l -a -e %s -e libspofycore.so /proc/[0-9]*/cmdline 2>/dev/null); do echo \"@@$f\"; cat $f 2>/dev/null; echo; done" % PACKAGE
    raw = subprocess.run(["adb", "exec-out", listing], capture_output=True, timeout=60).stdout
    out = {}
    for pid, body in re.findall(rb"@@/proc/(\d+)/cmdline\r?\n([^@]*)", raw):
        args = body.strip(b"\r\n\0").replace(b"\0", b" ").decode(errors="replace").strip()
        if args in (PACKAGE, PACKAGE + ":core") or args.split(" ")[0].endswith("libspofycore.so"):
            out[int(pid)] = args
    return out


def flight(on):
    if phone["sdk"] >= 30:
        shell("cmd connectivity airplane-mode %s" % ("enable" if on else "disable"))
    else:
        shell("settings put global airplane_mode_on %d; am broadcast -a android.intent.action.AIRPLANE_MODE --ez state %s" % (on, "true" if on else "false"))


def engines():
    return [pid for pid, args in processes().items() if args.endswith("libspofycore.so engine")]


def role(args):
    return "interface" if args == PACKAGE else "service" if args == PACKAGE + ":core" else "engine" if args.endswith(" engine") else "core"


def memory():
    total = {}
    for pid, args in processes().items():
        text = shell("dumpsys meminfo %d" % pid, limit=60)
        m = re.search(r"TOTAL PSS:\s+(\d+)", text) or re.search(r"TOTAL\s+(\d+)", text)
        if m:
            total[role(args)] = total.get(role(args), 0) + int(m.group(1)) // 1024
    return total


def cpu():
    total = {}
    for pid, args in processes().items():
        fields = shell("cat /proc/%d/stat" % pid).rsplit(")", 1)[-1].split()
        if len(fields) > 12:
            total[role(args)] = total.get(role(args), 0) + (int(fields[11]) + int(fields[12])) * 10
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
    learn()
    for attempt in (1, 2):
        code, out = run("adb", "install", "-r", "-g", APK, limit=240)
        if code == 0 and "Success" in out:
            break
        adb("wait-for-device")
    need("the app installs", code == 0 and "Success" in out, "attempt=%d %s" % (attempt, out[-300:]))
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
    kinds = list(processes().values())
    expect("the service runs in its own process", PACKAGE + ":core" in kinds and PACKAGE in kinds, str(kinds))

    s = ui.call("subscribe", url="https://example.com/sub/none")
    expect("the core resolves names and trusts the system certificates", not s["ok"] and s.get("code") in ("sub_status", "sub_format"), "code=%s %s" % (s.get("code"), s.get("error", "")[:200]))
    trust = {}
    for site in ("valid-isrgrootx1.letsencrypt.org", "valid-isrgrootx2.letsencrypt.org", "1.1.1.1/dns-query", "8.8.8.8/dns-query", "dns.google"):
        r = ui.call("subscribe", url="https://%s" % site)
        trust[site] = "not trusted" if "x509" in r.get("error", "") else r.get("code") or "ok"
    expect("the core trusts the certificates of the subscription, the servers and the DNS it uses",
           "not trusted" not in trust.values() and trust["valid-isrgrootx1.letsencrypt.org"] in ("sub_format", "sub_status"), str(trust))
    s = ui.call("subscribe", url="https://%s:8443/sub/EMULATORTOKEN" % GATE)
    need("subscribe", s["ok"], "ms=%d %s %s" % (s["ms"], s.get("code"), s.get("error", "")[:300]))
    m = ui.call("measure")
    servers = ([e for e in ui.events if e.get("ev") == "servers"] or [{"servers": []}])[-1]["servers"]
    expect("measure both servers", m["ok"] and len([x for x in servers if x["delayMs"] > 0]) == 2, "ms=%d delays=%s" % (m["ms"], {x["id"]: x["delayMs"] for x in servers}))
    def six():
        return shell("printf 'GET /ip HTTP/1.0\\r\\n\\r\\n' | %s%s -6 -w 3 fec0::2 8083 2>&1" % (phone["limit"] % 4 if phone["limit"] else "", phone["nc"]), limit=20)
    six()
    open6 = len(seen6)
    if not open6:
        print("this emulator gives the device no IPv6 path to the host: the check of IPv6 is left out", flush=True)
    direct = fetch(HOST, 8081, "/ip")
    expect("without the tunnel the site is reached directly", "site sees" in direct and not direct.endswith(INSIDE), direct[-40:])

    c = ui.call("connect", server="auto")
    first = ui.active()["server"]
    device = tunnel()
    need("connect", c["ok"] and first in SERVERS and device != "", "ms=%d server=%s %s %s" % (c["ms"], first, c.get("code"), c.get("error", "")[:300]))
    body = fetch(HOST, 8081, "/ip")
    need("another app's traffic goes through the tunnel", body.endswith("site sees " + INSIDE) and accepted(first, "%s:8081" % HOST) > 0, "body=%s accepted=%d" % (body[-60:], accepted(first, "%s:8081" % HOST)))
    if open6:
        six()
        expect("IPv6 from another app goes into the tunnel, not past it", len(seen6) == open6 and accepted(first, "fec0::2") > 0,
               "direct before the tunnel=%d, direct with it=%d, through the server=%d" % (open6, len(seen6) - open6, accepted(first, "fec0::2")))
    leaked, flowing = outside(), len(seen)
    loop = "while [ -e /data/local/tmp/noise ]; do printf 'GET /ip HTTP/1.0\\r\\n\\r\\n' | %s >/dev/null 2>&1; sleep 0.2; done\n" % piped(2, HOST, 8081)
    run("adb", "shell", "cat > /data/local/tmp/noise.sh", data=loop.encode())
    shell("touch /data/local/tmp/noise; nohup sh /data/local/tmp/noise.sh >/dev/null 2>&1 &")
    time.sleep(5)
    expect("background requests from another app are running", len(seen) >= flowing + 3, "requests=%d" % (len(seen) - flowing))
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
    def probed():
        out = []
        for _ in range(8):
            ui.call("check")
            out.append(ui.active()["delayMs"])
            time.sleep(0.13)
        return out
    rounds, outside_ms = probed(), []
    owner = shell("stat -c %%u %s" % DATA).strip()
    kick = "while [ -e /data/local/tmp/kick ]; do %s </dev/null >/dev/null 2>&1; done\n" % piped(1, GATE, 8082)
    run("adb", "shell", "cat > /data/local/tmp/kick.sh", data=kick.encode())
    alone = shell("su %s,%s,3003 sh -c '%s </dev/null; echo rc=$?' 2>&1" % (owner, owner, piped(2, GATE, 8082)), limit=20).strip()
    sent, began = len(kicks), time.time()
    shell("touch /data/local/tmp/kick; nohup su %s,%s,3003 sh /data/local/tmp/kick.sh >/dev/null 2>&1 &" % (owner, owner))
    time.sleep(1)
    kicked = probed()
    rate = (len(kicks) - sent) / (time.time() - began)
    shell("rm -f /data/local/tmp/kick")
    print("delay through the tunnel, ms: %s; the same while %d connections a second leave the device past the tunnel: %s; one such connection: %s"
          % (rounds, rate, kicked, alone[-60:]), flush=True)
    for _ in range(3):
        ui.call("measure")
        servers = ([e for e in ui.events if e.get("ev") == "servers"] or [{"servers": []}])[-1]["servers"]
        outside_ms.append(sorted(x["delayMs"] for x in servers))
    print("delay outside the tunnel while it is up, ms: %s; device clock %s at host %s" % (outside_ms, shell("date +%H:%M:%S.%N")[:12], stamp()), flush=True)

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
    alive = list(processes().values())
    expect("the interface is closed: the tunnel keeps working", through() and PACKAGE + ":core" in alive, "processes=%s" % alive)
    shell("am start -W -n %s/.MainActivity" % PACKAGE, limit=120)
    time.sleep(4)
    screen("4-reopened")
    expect("the interface is opened again", PACKAGE in list(processes().values()))

    c = ui.call("connect", server="auto")
    held = ui.active()["server"]
    remote[held].send_signal(signal.SIGSTOP)
    began = time.time()
    moved = until(lambda: ui.active()["state"] == "connected" and ui.active()["server"] != held and through(), 60, 0.2)
    remote[held].send_signal(signal.SIGCONT)
    expect("a server went silent under traffic: the app moved to the other one by itself on the same tunnel", c["ok"] and bool(moved) and tunnel() == device,
           "s=%.1f state=%s server=%s" % (time.time() - began, ui.active()["state"], ui.active()["server"]))
    if phone["sdk"] >= 26:
        second = linked("MOBILE")
        shell("svc wifi disable")
        began = time.time()
        if second:
            moved = until(lambda: ui.active()["state"] == "connected" and through(), 60)
            expect("Wi-Fi is off: traffic goes on over the mobile network on the same tunnel", bool(moved) and tunnel() == device, "s=%.1f state=%s device=%s" % (time.time() - began, ui.active()["state"], tunnel()))
        else:
            off = until(lambda: ui.active()["state"] == "offline", 20, 0.1)
            expect("Wi-Fi is off and there is no other network: the state says so and the tunnel stays", bool(off) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
        shell("svc wifi enable")
        began = time.time()
        again = until(lambda: linked("WIFI") and ui.active()["state"] == "connected" and through(), 90)
        expect("Wi-Fi is back: connected on the same tunnel", bool(again) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
    else:
        print("the emulator of this Android version has only a mobile network: the steps with Wi-Fi are left out", flush=True)
    flight(True)
    began = time.time()
    off = until(lambda: ui.active()["state"] == "offline", 40, 0.1)
    expect("no network: the state says so and the tunnel stays", bool(off) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
    screen("5-offline")
    flight(False)
    began = time.time()
    on = until(lambda: ui.active()["state"] == "connected" and through(), 120)
    expect("the network is back: connected again without being asked", bool(on) and tunnel() == device, "s=%.1f state=%s" % (time.time() - began, ui.active()["state"]))
    shell("rm -f /data/local/tmp/noise")
    time.sleep(3)
    expect("nothing reached the site past the tunnel while it was up", outside() == leaked, "requests=%d past=%d" % (len(seen), outside() - leaked))

    get = "printf 'GET %%s HTTP/1.0\\r\\nHost: %%s\\r\\n\\r\\n' \"$3\" \"$1\" | %s\n" % piped(8, '"$1"', '"$2"')
    run("adb", "shell", "cat > /data/local/tmp/get.sh", data=get.encode())
    mine = as_shell()
    expect("a second app, with its own user, is in the tunnel as well", mine.endswith("site sees " + INSIDE), mine[-40:])

    def chosen(value):
        before = tunnel()
        began = time.time()
        r = ui.call("set", key="apps", value=value)
        new = until(lambda: tunnel() not in ("", before) and ui.active()["state"] == "connected", 30, 0.1)
        return r["ok"] and bool(new), time.time() - began

    ok, took = chosen({"mode": "except", "list": ["com.android.shell"]})
    mine, rest = as_shell(), fetch(HOST, 8081, "/ip")
    expect("an app taken out of the VPN goes past the tunnel and the rest stay inside", ok and "site sees" in mine and not mine.endswith(INSIDE) and rest.endswith("site sees " + INSIDE),
           "s=%.1f app=%s rest=%s device=%s" % (took, mine[-24:], rest[-24:], tunnel()))
    run("adb", "push", os.path.join(os.path.dirname(CORE), "bind"), "/data/local/tmp/bind")
    shell("chmod 755 /data/local/tmp/bind")
    bound = shell("su 2000,2000,3003 /data/local/tmp/bind %s %s 8081 /ip" % (tunnel(), HOST), limit=30)
    if "operation not permitted" in bound:
        print("     this kernel does not let an app bind to an interface: the way past the app list through the tunnel does not exist here", flush=True)
    else:
        expect("an app taken out of the VPN cannot reach anything through the tunnel by binding to it", "site sees" not in bound, bound.strip()[-80:])
    ok, took = chosen({"mode": "only", "list": ["com.android.shell"]})
    mine, rest = as_shell(), fetch(HOST, 8081, "/ip")
    expect("only the chosen app is in the VPN: it goes through the tunnel and the rest go past it", ok and mine.endswith("site sees " + INSIDE) and "site sees" in rest and not rest.endswith(INSIDE),
           "s=%.1f app=%s rest=%s device=%s" % (took, mine[-24:], rest[-24:], tunnel()))
    ok, took = chosen(None)
    mine = as_shell()
    expect("the choice is cleared: every app is in the VPN again", ok and mine.endswith("site sees " + INSIDE) and through(), "s=%.1f app=%s device=%s" % (took, mine[-24:], tunnel()))

    d = ui.call("disconnect")
    gone = until(lambda: tunnel() == "", 10)
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("disconnect removes the tunnel and the notification", d["ok"] and bool(gone) and "isForeground=true" not in service, "ms=%d" % d["ms"])
    time.sleep(1)
    screen("6-disconnected")
    print("memory when idle, MB: %s" % memory(), flush=True)
    opened = tap("Настройки") and tap("Приложения") and tap("Все, кроме")
    marked = tap("Chrome", "Camera", "Calendar", "Clock", "Settings") if opened else ""
    time.sleep(2)
    screen("10-apps")
    saved = ui.call("hello")["snapshot"]["settings"].get("apps") or {}
    expect("the apps screen lists the installed apps and a tap on one reaches the core", bool(marked) and saved.get("mode") == "except" and len(saved.get("list", [])) == 1,
           "tapped=%s saved=%s" % (marked, saved))
    ui.call("set", key="apps", value=None)
    shell("input keyevent KEYCODE_BACK")
    time.sleep(1)
    shell("input keyevent KEYCODE_BACK")
    time.sleep(1)
    state = subprocess.run(["adb", "exec-out", "cat %s/files/state/state" % DATA], capture_output=True, timeout=60).stdout
    expect("saved state has nothing in the clear", b"EMULATORTOKEN" not in state and b"outbounds" not in state and len(state) > 400, "bytes=%d" % len(state))
    key = shell("cat %s/files/key | wc -c" % DATA)
    expect("the key file holds only the wrapped key", key.strip() == "60", "bytes=%s" % key.strip())
    ui.s.close()

    shell("am force-stop %s" % PACKAGE)
    time.sleep(1)
    begun = shell("am %s -a android.net.VpnService -n %s/.CoreService" % ("start-foreground-service" if phone["sdk"] >= 26 else "startservice", PACKAGE))
    began = time.time()
    up = until(lambda: tunnel() != "" and through(), 40, 0.2)
    alive = list(processes().values())
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("started by the system without the interface: the VPN comes up by itself", bool(up) and PACKAGE not in alive and "isForeground=true" in service,
           "s=%.1f processes=%d %s" % (time.time() - began, len(alive), begun[-120:]))
    later = Channel(9099)
    d = later.call("disconnect")
    gone = until(lambda: tunnel() == "" and "isForeground=true" not in shell("dumpsys activity services %s/.CoreService" % PACKAGE), 10)
    expect("disconnect after a start by the system removes the tunnel and the notification", d["ok"] and bool(gone))
    later.s.close()

    tile = "%s/%s.QuickTile" % (PACKAGE, PACKAGE)
    shell("cmd statusbar add-tile %s" % tile)
    time.sleep(2)
    shell("cmd statusbar click-tile %s" % tile)
    began = time.time()
    up = until(lambda: tunnel() != "" and through(), 40, 0.2)
    took = time.time() - began
    shell("cmd statusbar expand-settings")
    time.sleep(2)
    shell("input swipe 60 320 300 320 200")
    time.sleep(1)
    screen("7-tile")
    shell("cmd statusbar collapse")
    expect("the quick settings tile turns the VPN on", bool(up), "s=%.1f" % took)
    before = cpu()
    time.sleep(30)
    after = cpu()
    print("30 s with the VPN on and the screen idle, CPU ms: %s" % {k: after.get(k, 0) - before.get(k, 0) for k in after}, flush=True)
    shell("cmd statusbar click-tile %s" % tile)
    gone = until(lambda: tunnel() == "", 15)
    expect("the same tile turns it off", bool(gone))

    shell("am force-stop %s" % PACKAGE)
    before = len(asked)
    shell("am start -W -a android.intent.action.VIEW -d 'spofy://add/https://%s:8443/sub/OTHERTOKEN' %s" % (GATE, PACKAGE), limit=120)
    time.sleep(8)
    screen("8-link")
    opened = list(processes().values())
    expect("a link opens the app and adds nothing by itself", PACKAGE in opened and "/sub/OTHERTOKEN" not in asked[before:], "asked=%s" % asked[before:])

    by_system = "am %s -a android.net.VpnService -n %s/.CoreService" % ("start-foreground-service" if phone["sdk"] >= 26 else "startservice", PACKAGE)
    shell("am force-stop %s" % PACKAGE)
    panel["ended"] = True
    shell("am start -W -n %s/.MainActivity" % PACKAGE, limit=120)
    time.sleep(3)
    last = Channel(9099)
    r = last.call("refresh")
    snap = last.call("hello")["snapshot"]
    c = last.call("connect", server="auto")
    time.sleep(2)
    screen("9-ended")
    labels = shown()
    expect("a subscription that ended: no servers, the reason is known, connecting is refused, the screen leads to the payment",
           r["ok"] and snap["subscription"]["stop"] == "ended" and not snap["servers"] and not c["ok"] and c.get("code") == "sub_ended"
           and "Подписка закончилась" in labels and "Продлить подписку" in labels and tunnel() == "",
           "stop=%s servers=%d connect=%s labels=%s" % (snap["subscription"]["stop"], len(snap["servers"]), c.get("code"), labels[:200]))
    last.s.close()
    shell("am force-stop %s" % PACKAGE)
    shell(by_system)
    told = until(lambda: "Подписка закончилась" in shell("dumpsys notification --noredact"), 40)
    service = shell("dumpsys activity services %s/.CoreService" % PACKAGE)
    expect("started by the system with an ended subscription: the service steps down and a notice says why",
           bool(told) and "isForeground=true" not in service and tunnel() == "", re.sub(r"\s+", " ", service)[:160])
    panel["ended"] = False
    shell(by_system)
    up = until(lambda: tunnel() != "" and through(), 40, 0.2)
    expect("after the payment a start by the system brings the VPN up without the app being opened, and the notice is gone",
           bool(up) and "Подписка закончилась" not in shell("dumpsys notification --noredact"))
    last = Channel(9099)
    last.call("disconnect")
    until(lambda: tunnel() == "", 10)
    last.s.close()

    if RELEASE:
        adb("uninstall", PACKAGE, limit=120)
        code, out = run("adb", "install", "-g", RELEASE, limit=300)
        need("the release build installs", code == 0 and "Success" in out, out[-300:])
        times = started_in()
        time.sleep(3)
        screen("9-release")
        usage = memory()
        expect("the release build starts and its core listens", len(times) == 5 and "core.sock" in shell("ls %s/files" % DATA), "ms=%s" % times)
        print("release build: apk %.1f MB, installed %.1f MB, cold start %s ms with the device doing a fixed loop in %d ms, memory %s MB"
              % (os.path.getsize(RELEASE) / 1e6, installed(), times, speed(), usage), flush=True)


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
