import base64, hashlib, http.server, json, os, ssl, subprocess, sys, threading, time, urllib.request

BODY = open("/w/sub.json", "rb").read()
ETAG = 'W/"%s"' % hashlib.sha256(BODY).hexdigest()[:16]
HITS = {"full": 0, "unchanged": 0}


class Panel(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/sub/LABTOKEN":
            self.send_response(404)
            self.end_headers()
            return
        usage = "upload=1; download=%d; total=1000000; expire=1798761600" % (HITS["full"] + HITS["unchanged"])
        if self.headers.get("If-None-Match") == ETAG:
            HITS["unchanged"] += 1
            self.send_response(304)
            self.send_header("Subscription-Userinfo", usage)
            self.end_headers()
            return
        HITS["full"] += 1
        self.send_response(200)
        for k, v in (("Content-Type", "application/json"), ("Etag", ETAG), ("Profile-Update-Interval", "1"),
                     ("Subscription-Userinfo", usage), ("Profile-Title", "Lab")):
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(BODY)))
        self.end_headers()
        self.wfile.write(BODY)

    def log_message(self, *a):
        pass


srv = http.server.ThreadingHTTPServer(("127.0.0.1", 8443), Panel)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain("/tmp/tls/cert.pem", "/tmp/tls/key.pem")
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
threading.Thread(target=srv.serve_forever, daemon=True).start()


class Core:
    def __init__(self):
        self.p = subprocess.Popen(["/x/spofy-core", "serve", "-state", "/tmp/st", "-guard"], stdin=subprocess.PIPE,
                                  stdout=subprocess.PIPE, stderr=open("/tmp/serve.err", "ab"), text=True, bufsize=1)
        self.n = 0
        self.events = []

    def call(self, op, **fields):
        self.n += 1
        began = time.time()
        self.p.stdin.write(json.dumps({"id": self.n, "op": op, **fields}) + "\n")
        while True:
            line = self.p.stdout.readline()
            if not line:
                raise SystemExit("core closed the channel during " + op)
            m = json.loads(line)
            if m.get("re") == self.n:
                m["ms"] = int((time.time() - began) * 1000)
                return m
            self.events.append(m)

    def states(self):
        out = [e["connection"]["state"] + (":" + e["connection"]["server"] if e["connection"]["server"] else "")
               for e in self.events if e.get("ev") == "connection"]
        self.events = []
        return " ".join(out)

    def close(self):
        self.p.stdin.close()
        return self.p.wait(timeout=15)


def get(url, limit=8):
    r = subprocess.run(["curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "-m", str(limit), url], capture_output=True, text=True)
    return r.stdout


def rss(pid):
    for line in open("/proc/%d/status" % pid):
        if line.startswith("VmRSS"):
            return int(line.split()[1]) // 1024
    return 0


host_ip = subprocess.run(["curl", "-s", "-m", "8", "https://api.ipify.org"], capture_output=True, text=True).stdout
c = Core()
hello = c.call("hello")
print("hello: ok=%s servers=%d state=%s ms=%d" % (hello["ok"], len(hello["snapshot"]["servers"]), hello["snapshot"]["connection"]["state"], hello["ms"]))
bad = c.call("subscribe", url="https://sub.lab.test:8443/sub/WRONG")
print("subscribe_wrong_address: code=%s" % bad.get("code"))
s = c.call("subscribe", url="https://sub.lab.test:8443/sub/LABTOKEN")
servers = [e for e in c.events if e.get("ev") == "servers"]
print("subscribe: ok=%s ms=%d servers=%d" % (s["ok"], s["ms"], len(servers[-1]["servers"]) if servers else -1))
c.events = []
m = c.call("measure")
answered = [x for x in [e for e in c.events if e.get("ev") == "servers"][-1]["servers"] if x["delayMs"] > 0]
print("measure: ok=%s ms=%d answered=%d fastest=%s" % (m["ok"], m["ms"], len(answered), min(answered, key=lambda x: x["delayMs"])["id"] if answered else "-"))
c.events = []
r = c.call("connect", server="auto")
print("connect_auto: ok=%s ms=%d states=[%s] site_204=%s exit_differs=%s" % (
    r["ok"], r["ms"], c.states(), get("https://www.gstatic.com/generate_204"),
    "yes" if subprocess.run(["curl", "-s", "-m", "8", "https://api.ipify.org"], capture_output=True, text=True).stdout not in ("", host_ip) else "NO"))
t = c.call("traffic")
print("traffic: up=%d down=%d" % (t["traffic"]["up"], t["traffic"]["down"]))
busy = c.call("measure")
print("measure_while_connected: code=%s" % busy.get("code"))
subprocess.run(["pkill", "-9", "-f", "spofy-core engine"])
time.sleep(1.5)
k = c.call("check")
print("after_engine_kill: check_ok=%s states=[%s] site_204=%s" % (k["ok"], c.states(), get("https://www.gstatic.com/generate_204")))
f = c.call("refresh")
print("refresh: ok=%s changed=%s ms=%d panel_full=%d panel_unchanged=%d download_counter=%s" % (
    f["ok"], f.get("changed"), f["ms"], HITS["full"], HITS["unchanged"],
    [e for e in c.events if e.get("ev") == "subscription"][-1]["subscription"]["download"]))
c.call("set", key="killSwitch", value=True)
pin = c.call("connect", server="de")
print("switch_to_de: ok=%s ms=%d states=[%s] site_204=%s" % (pin["ok"], pin["ms"], c.states(), get("https://www.gstatic.com/generate_204")))
print("memory: service_mb=%d" % rss(c.p.pid))
d = c.call("disconnect")
print("disconnect: ok=%s ms=%d direct_204=%s tun=%s" % (d["ok"], d["ms"], get("https://www.gstatic.com/generate_204"),
      "PRESENT" if os.path.exists("/sys/class/net/spofy0") else "gone"))
print("exit_code=%d" % c.close())

c = Core()
h = c.call("hello")
snap = h["snapshot"]
print("restart: servers=%d known_delays=%d choice=%s settings=%s subscription=%s" % (
    len(snap["servers"]), len([x for x in snap["servers"] if x["delayMs"] > 0]), snap["connection"]["choice"],
    json.dumps(snap["settings"]), "kept" if snap["subscription"] else "LOST"))
r = c.call("connect", server="auto")
print("connect_auto_again: ok=%s ms=%d states=[%s] site_204=%s panel_hits=%d" % (r["ok"], r["ms"], c.states(), get("https://www.gstatic.com/generate_204"), HITS["full"] + HITS["unchanged"]))
c.p.kill()
c.p.wait()
time.sleep(0.5)
left = len([x for x in subprocess.run(["ps", "-C", "spofy-core", "-o", "stat="], capture_output=True, text=True).stdout.split() if not x.startswith("Z")])
print("service_killed: processes_left=%s tun=%s direct_204=%s" % (left, "PRESENT" if os.path.exists("/sys/class/net/spofy0") else "gone", get("https://www.gstatic.com/generate_204")))
clear = sum(1 for w in (b"outbounds", b"LABTOKEN", b"sub.lab.test") if w in open("/tmp/st/state", "rb").read())
print("state_file: bytes=%d words_in_the_clear=%d" % (os.path.getsize("/tmp/st/state"), clear))
err = open("/tmp/serve.err").read()
kinds = {}
for line in err.splitlines():
    k = " ".join(line.split()[2:6])
    kinds[k] = kinds.get(k, 0) + 1
print("stderr_lines=%d panics=%d errors=%d kinds=%s" % (len(err.splitlines()), err.count("panic"), err.count("[Error]"), sorted(kinds.items(), key=lambda x: -x[1])[:4]))
