import base64, hashlib, ipaddress, json, sys

src, dst = sys.argv[1], sys.argv[2]
seen, real = {}, {}

PASS = {
    "protocol", "tag", "network", "security", "flow", "fingerprint", "alpn", "mode", "domainStrategy",
    "nonIPQuery", "type", "outboundTag", "balancerTag", "fallbackTag", "selector", "subjectSelector",
    "port", "queryStrategy", "loglevel", "destOverride", "ipPool", "connectivity", "interval", "timeout",
    "congestion", "inboundTag", "remarks", "domainMatcher",
}
RESOLVERS = {
    "fakedns", "localhost", "1.1.1.1", "1.0.0.1", "8.8.8.8", "8.8.4.4", "9.9.9.9", "77.88.8.8", "77.88.8.1",
    "https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query", "https://dns.google/dns-query",
    "https://cloudflare-dns.com/dns-query",
}
PROBE = "http://www.gstatic.com/generate_204"
MATCHERS = ("domain", "full", "keyword", "regexp", "dotless")
PORTS = (443, 8443)


def pos(kind, v):
    if len(v) >= 5:
        real[v] = kind
    t = seen.setdefault(kind, {})
    return t.setdefault(v, len(t) + 1)


def host(v):
    if not v:
        return v
    try:
        ipaddress.ip_address(v)
    except ValueError:
        return "h%d.example.test" % pos("host", v)
    return "192.0.2.%d" % (pos("ip", v) + 9)


def domain(v):
    kind, sep, body = v.partition(":")
    if not sep:
        return "d%d.example.test" % pos("domain", v)
    if kind in ("geosite", "ext"):
        return v
    if kind not in MATCHERS:
        sys.exit("unknown domain matcher: " + kind)
    n = pos("domain", v)
    if kind == "keyword":
        return "keyword:kw%d" % n
    if kind == "regexp":
        return "regexp:^r%d\\.example\\.test$" % n
    return "%s:d%d.example.test" % (kind, n)


def net(v):
    if v.startswith(("geoip:", "ext:")) or v in RESOLVERS:
        return v
    n = ipaddress.ip_network(v, strict=False)
    if n.is_private or n.is_loopback or n.is_link_local or n.is_multicast or n.is_unspecified:
        return v
    return "198.51.100.%d" % pos("net", v)


def forget(o):
    if isinstance(o, dict):
        for v in o.values():
            forget(v)
    elif isinstance(o, list):
        for v in o:
            forget(v)
    elif isinstance(o, str) and len(o) >= 10:
        real[o] = "tuning"


def leaf(path, key, v):
    if not isinstance(v, str):
        if key == "port" and path.startswith("outbounds") and v not in PORTS:
            return 2053
        return v
    if v == "":
        return v
    if key == "id" or (key == "auth" and v != "noauth"):
        return "00000000-0000-4000-8000-%012d" % pos("uuid", v)
    if key == "auth":
        return v
    if key == "password":
        return "password-%d" % pos("password", v)
    if key == "publicKey":
        return base64.urlsafe_b64encode(hashlib.sha256(b"fixture-key-%d" % pos("key", v)).digest()).decode().rstrip("=")
    if key == "shortId":
        return "%016x" % pos("short", v)
    if key in ("address", "serverName", "host", "authority"):
        return host(v)
    if key == "path":
        return v if len(v) <= 1 else "/p%d" % pos("path", v)
    if key == "serviceName":
        return "svc"
    if key == "encryption":
        return "none"
    if key in ("domain", "domains"):
        return domain(v)
    if key == "ip":
        return net(v)
    if key == "ruleTag":
        return "rule-%d" % pos("rule", v)
    if key == "listen":
        return "127.0.0.1"
    if key == "destination":
        return PROBE
    if key in PASS:
        return v
    sys.exit("unknown field: %s = add a rule to tools/mkfixture.py" % path)


def walk(o, path="", key=None):
    if isinstance(o, dict):
        out = {}
        for k, v in o.items():
            p = path + "." + k if path else k
            if k == "extra":
                forget(v)
                out[k] = {"xPaddingBytes": "100-1000"}
            elif k == "finalmask":
                out[k] = mask(v, p)
            elif k in ("domain", "domains", "ip") and isinstance(v, list):
                forget(v[5:])
                out[k] = walk(v[:5], p, k)
            else:
                out[k] = walk(v, p, k)
        return out
    if isinstance(o, list):
        return [walk(v, path, key) for v in o]
    return leaf(path, key, o)


def mask(o, path):
    out = {}
    for k, v in o.items():
        if k == "tcp":
            forget(v)
            out[k] = [
                {"type": "fragment", "settings": {"packets": "tlshello" if (m.get("settings") or {}).get("packets") == "tlshello" else "1-3", "length": "100-200", "delay": "10-20"}}
                for m in v
            ]
        elif k == "quicParams":
            out[k] = {a: b for a, b in v.items() if not isinstance(b, str) or a == "congestion"}
            forget([b for a, b in v.items() if isinstance(b, str) and a != "congestion"])
        else:
            sys.exit("unknown field: %s.%s = add a rule to tools/mkfixture.py" % (path, k))
    return out


def dns(o):
    out = {}
    for k, v in o.items():
        if k == "hosts":
            out[k] = {}
            for name, target in v.items():
                targets = target if isinstance(target, list) else [target]
                fake = [t if t in ("0.0.0.0", "127.0.0.1") else host(t) for t in targets]
                out[k]["block%d.example.test" % pos("blocked", name)] = fake if isinstance(target, list) else fake[0]
        elif k == "servers":
            out[k] = [resolver(s) for s in v]
        else:
            out[k] = walk(v, "dns." + k, k)
    return out


def resolver(s):
    if isinstance(s, str):
        if s not in RESOLVERS:
            sys.exit("unknown resolver in dns.servers: add it to RESOLVERS or a rule")
        return s
    out = {}
    for k, v in s.items():
        if k == "address":
            if v not in RESOLVERS:
                sys.exit("unknown resolver in dns.servers: add it to RESOLVERS or a rule")
            out[k] = v
        else:
            out.update(walk({k: v}, "dns.servers"))
    return out


configs = json.load(open(src + "/sub.json"))
out = []
for c in configs:
    plain = [
        i for i, o in enumerate(c["outbounds"])
        if o.get("protocol") == "vless" and (o.get("streamSettings") or {}).get("security") == "none"
    ]
    f = {k: (dns(v) if k == "dns" else walk(v, k, k)) for k, v in c.items()}
    for i in plain:
        for v in f["outbounds"][i]["settings"]["vnext"]:
            v["address"] = "10.203.0.%d" % (i + 10)
    out.append(f)

keep = {}
for line in open(src + "/sub.headers", encoding="utf-8", errors="replace"):
    if ":" not in line:
        continue
    k, v = line.split(":", 1)
    keep[k.strip().lower()] = v.strip()
for k in ("support-url", "profile-web-page-url", "announce", "etag", "subscription-userinfo"):
    if len(keep.get(k, "")) >= 5:
        real[keep[k]] = "header"
headers = {
    "Content-Type": "application/json; charset=utf-8",
    "Profile-Title": keep.get("profile-title", ""),
    "Subscription-Userinfo": "upload=1048576; download=452716590; total=107374182400; expire=1798761600",
    "Profile-Update-Interval": "1",
    "Support-Url": "https://support.example.test",
    "Profile-Web-Page-Url": "https://sub.example.test/TESTSHORTUUID",
    "Announce": "base64:" + base64.b64encode("Тестовое объявление".encode()).decode(),
    "Etag": 'W/"fixture-1"',
    "X-Hwid-Active": "true",
}

body = json.dumps(out, ensure_ascii=False, separators=(",", ":"))
head = json.dumps(headers, ensure_ascii=False, indent=1)
leaks = sorted(v for v in real if v in body or v in head)
if leaks:
    sys.exit("fixture still contains %d real values of kinds %s" % (len(leaks), sorted({real[v] for v in leaks})))
for path in sys.argv[3:]:
    try:
        text = open(path, encoding="utf-8").read()
    except (UnicodeDecodeError, OSError):
        continue
    found = sorted({real[v] for v in real if v in text})
    if found:
        sys.exit("%s contains real values of kinds %s" % (path, found))
open(dst + "/sub.json", "w").write(body)
open(dst + "/headers.json", "w").write(head)
print(len(out), "configs,", len(body), "bytes; replaced", len(real), "real values, 0 left")
