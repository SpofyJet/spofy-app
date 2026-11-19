import base64, json, os, shutil, subprocess, sys, tempfile, threading, time

CORE = os.path.abspath(sys.argv[1])
HERE = os.path.dirname(os.path.abspath(__file__))
WORK = tempfile.mkdtemp(prefix="spofy-hermetic-")
os.chmod(WORK, 0o755)
SPACES = ["hub", "client", "a", "b", "site"]
SERVERS = {"a": "10.77.0.11", "b": "10.77.0.12"}
CLIENT = "10.77.0.2"
USER = "00000000-0000-4000-8000-000000000001"
FOREIGN = ("www.gstatic.com", "cp.cloudflare.com", "foreign.test")
failed = []
started = []


class Abort(Exception):
    pass


def run(*cmd, check=True, limit=30):
    r = subprocess.run(cmd, capture_output=True, text=True, timeout=limit)
    if check and r.returncode != 0:
        raise SystemExit("command failed: %s\n%s" % (" ".join(cmd), r.stderr.strip()))
    return r.stdout


def inside(space, *cmd, **kw):
    return run("ip", "netns", "exec", space, *cmd, **kw)


def spawn(space, cmd, **kw):
    p = subprocess.Popen(["ip", "netns", "exec", space] + cmd, **kw)
    started.append(p)
    return p


def expect(name, ok, detail=""):
    print("%s %s %s" % ("ok  " if ok else "FAIL", name, detail), flush=True)
    if not ok:
        failed.append(name)
    return ok


def need(name, ok, detail=""):
    if not expect(name, ok, detail):
        raise Abort()


def fetch(url, limit=5):
    if os.path.exists(WORK + "/body"):
        os.remove(WORK + "/body")
    r = subprocess.run(["ip", "netns", "exec", "client", "curl", "-s", "-m", str(limit), "-o", WORK + "/body", "-w", "%{http_code}", url], capture_output=True, text=True)
    body = open(WORK + "/body").read() if os.path.exists(WORK + "/body") else ""
    return r.stdout, body


def counter(name):
    out = inside("hub", "nft", "list", "counter", "inet", "hub", name)
    return int(out.split("packets")[1].split()[0])


def questions():
    return [l.split() for l in open(WORK + "/dns.log").read().splitlines()]


def build():
    for s in SPACES:
        run("ip", "netns", "add", s)
        run("ip", "-n", s, "link", "set", "lo", "up")
    run("ip", "-n", "hub", "link", "add", "br0", "type", "bridge")
    run("ip", "-n", "hub", "addr", "add", "10.77.0.1/24", "dev", "br0")
    run("ip", "-n", "hub", "link", "set", "br0", "up")
    for s, addr in [("client", CLIENT)] + list(SERVERS.items()):
        run("ip", "-n", "hub", "link", "add", "v-" + s, "type", "veth", "peer", "name", "eth0", "netns", s)
        run("ip", "-n", "hub", "link", "set", "v-" + s, "master", "br0", "up")
        run("ip", "-n", s, "addr", "add", addr + "/24", "dev", "eth0")
        run("ip", "-n", s, "link", "set", "eth0", "up")
        run("ip", "-n", s, "route", "add", "default", "via", "10.77.0.1")
    run("ip", "-n", "hub", "link", "add", "v-site", "type", "veth", "peer", "name", "eth0", "netns", "site")
    for addr in ("10.77.1.1/24", "10.77.2.1/24"):
        run("ip", "-n", "hub", "addr", "add", addr, "dev", "v-site")
    run("ip", "-n", "hub", "link", "set", "v-site", "up")
    for addr in ("10.77.1.2/24", "10.77.1.3/24", "10.77.2.2/24"):
        run("ip", "-n", "site", "addr", "add", addr, "dev", "eth0")
    run("ip", "-n", "site", "link", "set", "eth0", "up")
    run("ip", "-n", "site", "route", "add", "default", "via", "10.77.1.1")
    inside("hub", "sysctl", "-qw", "net.ipv4.ip_forward=1")
    for key in ("all", "default", "eth0"):
        inside("client", "sysctl", "-qw", "net.ipv4.conf.%s.rp_filter=2" % key)
    rules = """
table inet hub {
  counter foreign_direct { }
  counter dns_direct { }
  chain forward {
    type filter hook forward priority 0;
    ip saddr %s ip daddr 10.77.1.0/24 udp dport 53 counter name dns_direct accept
    ip saddr %s ip daddr 10.77.1.0/24 counter name foreign_direct drop
  }
}""" % (CLIENT, CLIENT)
    open(WORK + "/hub.nft", "w").write(rules)
    inside("hub", "nft", "-f", WORK + "/hub.nft")
    os.makedirs("/etc/netns/client", exist_ok=True)
    open("/etc/netns/client/resolv.conf", "w").write("nameserver 10.77.1.2\noptions timeout:2 attempts:1\n")


def config(name, addr):
    sniffing = {"enabled": True, "routeOnly": False, "destOverride": ["http", "tls"]}
    return {
        "remarks": "\U0001F3F3 (%s)" % name.upper(),
        "dns": {"servers": ["10.77.1.3"], "queryStrategy": "UseIPv4"},
        "inbounds": [{"tag": "socks", "port": 10808, "listen": "127.0.0.1", "protocol": "socks", "settings": {"udp": True, "auth": "noauth"}, "sniffing": sniffing}],
        "outbounds": [
            {"tag": "proxy", "protocol": "vless", "settings": {"vnext": [{"address": addr, "port": 443, "users": [{"id": USER, "encryption": "none"}]}]},
             "streamSettings": {"network": "tcp", "security": "none"}},
            {"tag": "direct", "protocol": "freedom"},
            {"tag": "block", "protocol": "blackhole"},
            {"tag": "dns-out", "protocol": "dns"},
        ],
        "routing": {"domainStrategy": "AsIs", "rules": [
            {"type": "field", "ip": ["10.77.1.3"], "outboundTag": "proxy"},
            {"type": "field", "port": "53", "outboundTag": "dns-out"},
            {"type": "field", "domain": ["domain:local.test"], "outboundTag": "direct"},
            {"type": "field", "ip": ["10.77.2.0/24"], "outboundTag": "direct"},
        ]},
    }


def serve_side():
    json.dump([config(n, a) for n, a in SERVERS.items()], open(WORK + "/sub.json", "w"), ensure_ascii=False)
    run("openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1", "-nodes", "-days", "1", "-keyout", WORK + "/key.pem",
        "-out", WORK + "/cert.pem", "-subj", "/CN=sub.local.test", "-addext", "subjectAltName=DNS:sub.local.test")
    spawn("site", [sys.executable, HERE + "/site.py", WORK], stderr=open(WORK + "/site.err", "w"))
    if not until(lambda: os.path.exists(WORK + "/site.ready"), 20, 0.05):
        raise SystemExit("the site did not start: " + open(WORK + "/site.err").read()[-600:])
    remote = {
        "log": {"loglevel": os.environ.get("HERMETIC_LOG", "warning")},
        "dns": {"servers": ["10.77.1.2"], "queryStrategy": "UseIPv4"},
        "inbounds": [{"tag": "in", "listen": "0.0.0.0", "port": 443, "protocol": "vless", "settings": {"clients": [{"id": USER}], "decryption": "none"},
                      "streamSettings": {"network": "tcp", "security": "none"}}],
        "outbounds": [{"protocol": "freedom", "settings": {"domainStrategy": "UseIPv4", "finalRules": [{"action": "allow", "ip": ["10.77.0.0/16"]}]}}],
    }
    for name in SERVERS:
        p = spawn(name, [CORE, "engine"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(WORK + "/server-%s.err" % name, "w"), text=True, bufsize=1)
        p.stdin.write(json.dumps({"id": 1, "op": "start", "config": remote}) + "\n")
        while True:
            line = p.stdout.readline()
            if not line:
                raise SystemExit("server %s did not start: %s" % (name, open(WORK + "/server-%s.err" % name).read()[-400:]))
            m = json.loads(line)
            if m.get("re") == 1:
                if not m.get("ok"):
                    raise SystemExit("server %s rejected its config: %s" % (name, line))
                break
        threading.Thread(target=drain, args=(p.stdout, WORK + "/server-%s.err" % name), daemon=True).start()


def drain(stream, path):
    with open(path, "a", buffering=1) as out:
        for line in stream:
            out.write(line)


def command(pid):
    try:
        return open("/proc/%d/cmdline" % pid, "rb").read().replace(b"\0", b" ").decode()
    except OSError:
        return ""


def children(pid):
    return [int(x) for x in run("pgrep", "-P", str(pid), check=False).split()]


class Core:
    def __init__(self):
        env = dict(os.environ, SPOFY_STATE_KEY=KEY, SSL_CERT_FILE=WORK + "/cert.pem")
        self.p = spawn("client", [CORE, "serve", "-state", WORK + "/state", "-guard"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                       stderr=open(WORK + "/core.err", "a"), text=True, bufsize=1, env=env)
        self.n = 0
        self.events = []

    def call(self, op, **fields):
        self.n += 1
        began = time.time()
        self.p.stdin.write(json.dumps({"id": self.n, "op": op, **fields}) + "\n")
        while True:
            line = self.p.stdout.readline()
            if not line:
                raise SystemExit("the core closed the channel during %s: %s" % (op, open(WORK + "/core.err").read()[-600:]))
            m = json.loads(line)
            if m.get("re") == self.n:
                m["ms"] = int((time.time() - began) * 1000)
                return m
            self.events.append(m)

    def states(self):
        out = [e["connection"]["state"] + (":" + e["connection"]["server"] if e["connection"]["server"] else "")
               for e in self.events if e.get("ev") == "connection"]
        self.events = []
        return out

    def active(self):
        return self.call("hello")["snapshot"]["connection"]

    def service(self):
        if " serve " in command(self.p.pid) and not command(self.p.pid).startswith("ip "):
            return self.p.pid
        return next(pid for pid in children(self.p.pid) if " serve " in command(pid))

    def engine(self):
        return [pid for pid in children(self.service()) if command(pid).rstrip().endswith(" engine")]


def alive(pattern):
    return [x for x in run("ps", "-eo", "stat=,args=", check=False).splitlines() if pattern in x and not x.strip().startswith("Z")]


def until(what, limit=30.0, step=0.2):
    end = time.time() + limit
    while time.time() < end:
        v = what()
        if v:
            return v
        time.sleep(step)
    return None


def scenario():
    code, _ = fetch("http://10.77.1.2/ip", 2)
    need("no direct path from the client to foreign sites", code != "200" and counter("foreign_direct") > 0, "code=%s" % code)
    core = Core()
    hello = core.call("hello")
    need("fresh start", hello["ok"] and hello["snapshot"]["servers"] == [])
    s = core.call("subscribe", url="https://sub.local.test/sub/HERMETICTOKEN")
    need("subscribe", s["ok"], "ms=%d %s" % (s["ms"], s.get("error", "")))
    m = core.call("measure")
    servers = ([e for e in core.events if e.get("ev") == "servers"] or [{"servers": []}])[-1]["servers"]
    need("measure both servers", m["ok"] and len([x for x in servers if x["delayMs"] > 0]) == 2, "ms=%d delays=%s" % (m["ms"], {x["id"]: x["delayMs"] for x in servers}))
    core.events = []
    inside("hub", "nft", "reset", "counter", "inet", "hub", "foreign_direct")
    before = len(questions())
    c = core.call("connect", server="auto")
    first = core.active()["server"]
    need("connect auto", c["ok"] and first in SERVERS, "ms=%d server=%s %s" % (c["ms"], first, c.get("error", "")))
    code, _ = fetch("http://www.gstatic.com/generate_204")
    expect("site by name through the tunnel", code == "204", "code=%s" % code)
    code, body = fetch("http://foreign.test/ip")
    expect("foreign site sees the server address", body == SERVERS.get(first), "seen=%s" % body)
    code, body = fetch("http://local.test/ip")
    expect("direct site sees the client address", body == CLIENT, "seen=%s" % body)
    t = core.call("traffic")
    expect("traffic is counted", t["traffic"]["down"] > 0 and t["traffic"]["up"] > 0, str(t["traffic"]))

    noise = spawn("client", ["sh", "-c", "while :; do curl -s -m 1 -o /dev/null http://10.77.1.2/ip; done"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(0.5)
    kills = 0
    for _ in range(3):
        for pid in core.engine():
            os.kill(pid, 9)
            kills += 1
        time.sleep(0.7)
    back = until(lambda: fetch("http://foreign.test/ip", 2)[0] == "200", 10)
    expect("engine killed three times: traffic is back", kills == 3 and bool(back), "kills=%d" % kills)
    expect("engine killed three times: nothing left the client directly", counter("foreign_direct") == 0, "packets=%d" % counter("foreign_direct"))

    first = core.active()["server"]
    other = [n for n in SERVERS if n != first][0]
    began = time.time()
    run("ip", "-n", "hub", "link", "set", "v-" + first, "down")

    def state(want, server=None):
        core.call("check")
        now = core.active()
        return now["state"] == want and (server is None or now["server"] == server)
    ok = until(lambda: state("connected", other), 40, 0.5)
    took = time.time() - began
    code, body = fetch("http://foreign.test/ip")
    expect("silent server is replaced", bool(ok) and body == SERVERS[other], "s=%.1f seen=%s" % (took, body))
    expect("switch: nothing left the client directly", counter("foreign_direct") == 0, "packets=%d" % counter("foreign_direct"))

    run("ip", "-n", "hub", "link", "set", "v-" + other, "down")
    stalled = until(lambda: state("stalled"), 60, 0.5)
    code, body = fetch("http://10.77.2.2/ip", 3)
    tun = inside("client", "ip", "-o", "link", "show", check=False)
    expect("every server silent: tunnel and block stay", bool(stalled) and "spofy0" in tun and counter("foreign_direct") == 0, "packets=%d" % counter("foreign_direct"))
    expect("every server silent: direct site still opens", body == CLIENT, "seen=%s" % body)
    began = time.time()
    for name in SERVERS:
        run("ip", "-n", "hub", "link", "set", "v-" + name, "up")
    again = until(lambda: state("connected"), 40, 0.5)
    expect("servers are back: connected again", bool(again), "s=%.1f server=%s" % (time.time() - began, core.active()["server"]))
    noise.kill()

    r = core.call("refresh")
    expect("refresh without changes", r["ok"] and r.get("changed") is False and "unchanged" in open(WORK + "/panel.log").read(), str({k: r.get(k) for k in ("ok", "changed", "code", "error")}))
    leaked = [l for l in questions()[before:] if l[0] == CLIENT]
    expect("no DNS question left the client directly while connected", not leaked and len(questions()) > before, "direct=%d of %d" % (len(leaked), len(questions()) - before))
    expect("nothing left the client directly while connected", counter("foreign_direct") == 0, "packets=%d" % counter("foreign_direct"))
    core.call("set", key="killSwitch", value=True)
    d = core.call("disconnect")
    tun = inside("client", "ip", "-o", "link", "show", check=False)
    tables = inside("client", "nft", "list", "tables", check=False)
    expect("disconnect leaves nothing", d["ok"] and "spofy0" not in tun and "spofy" not in tables, "ms=%d" % d["ms"])
    core.p.stdin.close()
    expect("the core exits when its channel closes", core.p.wait(timeout=15) == 0)

    core = Core()
    snap = core.call("hello")["snapshot"]
    need("restart keeps the state", len(snap["servers"]) == 2 and snap["subscription"] is not None and snap["settings"].get("killSwitch") is True
         and all(x["delayMs"] > 0 for x in snap["servers"]))
    c = core.call("connect")
    states = core.states()
    code, body = fetch("http://foreign.test/ip")
    expect("connect from memory", c["ok"] and "measuring" not in states and body in SERVERS.values(), "states=%s" % states)
    os.kill(core.service(), 9)
    gone = until(lambda: not alive("spofy-core serve") and len(alive("spofy-core engine")) == len(SERVERS), 10)
    tun = inside("client", "ip", "-o", "link", "show", check=False)
    tables = inside("client", "nft", "list", "tables", check=False)
    expect("service killed: engine, tunnel and block are gone", bool(gone) and "spofy0" not in tun and "spofy" not in tables)
    state_file = open(WORK + "/state/state", "rb").read()
    expect("saved state has nothing in the clear", not any(w in state_file for w in (b"outbounds", b"HERMETICTOKEN", USER.encode())), "bytes=%d" % len(state_file))
    logs = open(WORK + "/core.err").read()
    expect("no panic in the core", "panic" not in logs and "fatal error" not in logs)
    expect("the address of the subscription is not in the logs", "HERMETICTOKEN" not in logs)


def diagnose():
    checks = [
        ("client to server a", ["ip", "netns", "exec", "client", "curl", "-s", "-m", "3", "-o", "/dev/null", "-w", "%{http_code} %{errormsg}", "http://10.77.0.11:443/"]),
        ("server a to the site", ["ip", "netns", "exec", "a", "curl", "-s", "-m", "3", "-o", "/dev/null", "-w", "%{http_code} %{errormsg}", "http://10.77.1.2/generate_204"]),
        ("measure", ["ip", "netns", "exec", "client", CORE, "measure", "-sub", WORK + "/sub.json", "-timeout", "3s"]),
        ("client routes", ["ip", "-n", "client", "route"]),
        ("client links", ["ip", "-n", "client", "-o", "link"]),
    ]
    for name, cmd in checks:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        print("--- %s\n%s" % (name, (r.stdout + r.stderr).strip()[-1200:]))
    print("--- dns questions\n%s" % open(WORK + "/dns.log").read()[-600:])


def cleanup():
    for p in started:
        if p.poll() is None:
            p.kill()
    time.sleep(0.3)
    for s in SPACES:
        for pid in subprocess.run(["ip", "netns", "pids", s], capture_output=True, text=True).stdout.split():
            try:
                os.kill(int(pid), 9)
            except ProcessLookupError:
                pass
    time.sleep(0.3)
    for s in SPACES:
        subprocess.run(["ip", "netns", "del", s], capture_output=True)
    shutil.rmtree("/etc/netns/client", ignore_errors=True)
    shutil.rmtree(WORK, ignore_errors=True)


KEY = base64.b64encode(os.urandom(32)).decode()
began = time.time()
try:
    build()
    serve_side()
    scenario()
except Abort:
    pass
finally:
    if failed:
        for name in ("core.err", "site.err", "server-a.err", "server-b.err"):
            path = WORK + "/" + name
            if os.path.exists(path):
                print("--- %s\n%s" % (name, open(path).read()[-1500:]))
        diagnose()
    cleanup()
print("%s in %.0f s, %d failed" % ("HERMETIC OK" if not failed else "HERMETIC FAILED", time.time() - began, len(failed)))
sys.exit(1 if failed else 0)
