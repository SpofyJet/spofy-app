import hashlib, http.server, socket, socketserver, ssl, struct, sys, threading

WORK = sys.argv[1]
NAMES = {
    "www.gstatic.com": "10.77.1.2", "cp.cloudflare.com": "10.77.1.2", "foreign.test": "10.77.1.2",
    "local.test": "10.77.2.2", "sub.local.test": "10.77.2.2",
}


def published():
    body = open(WORK + "/sub.json", "rb").read()
    every = open(WORK + "/interval").read().strip()
    return body, every, 'W/"%s"' % hashlib.sha256(body + every.encode()).hexdigest()[:16]


asked = open(WORK + "/dns.log", "a", buffering=1)
hits = open(WORK + "/panel.log", "a", buffering=1)


def dns(s):
    while True:
        data, peer = s.recvfrom(1500)
        try:
            labels, at = [], 12
            while data[at]:
                labels.append(data[at + 1:at + 1 + data[at]].decode())
                at += 1 + data[at]
            name = ".".join(labels).lower()
            kind = struct.unpack(">H", data[at + 1:at + 3])[0]
            question = data[12:at + 5]
        except (IndexError, UnicodeDecodeError, struct.error):
            continue
        asked.write("%s %s %d\n" % (peer[0], name, kind))
        known = name in NAMES
        answer = b""
        if known and kind == 1:
            answer = b"\xc0\x0c" + struct.pack(">HHIH", 1, 1, 30, 4) + socket.inet_aton(NAMES[name])
        flags = 0x8180 if known else 0x8183
        s.sendto(data[:2] + struct.pack(">HHHHH", flags, 1, 1 if answer else 0, 0, 0) + question + answer, peer)


class Server(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def server_bind(self):
        socketserver.TCPServer.server_bind(self)
        self.server_name, self.server_port = "site", self.server_address[1]


class Site(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

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
            self.reply(200, self.client_address[0].encode())
        elif self.path == "/sub/HERMETICTOKEN" and self.server.server_port == 443:
            usage = ("Subscription-Userinfo", "upload=1; download=2; total=1000; expire=%s" % open(WORK + "/expire").read().strip())
            body, every, etag = published()
            if self.headers.get("If-None-Match") == etag:
                hits.write("unchanged\n")
                self.reply(304, headers=[usage])
            else:
                hits.write("full\n")
                self.reply(200, body, [("Content-Type", "application/json"), ("Etag", etag), usage, ("Profile-Update-Interval", every), ("Profile-Title", "Hermetic")])
        else:
            self.reply(404)

    def log_message(self, *a):
        pass


for address in ("10.77.1.2", "10.77.1.3"):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind((address, 53))
    threading.Thread(target=dns, args=(s,), daemon=True).start()
plain = Server(("0.0.0.0", 80), Site)
secure = Server(("10.77.2.2", 443), Site)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(WORK + "/cert.pem", WORK + "/key.pem")
secure.socket = ctx.wrap_socket(secure.socket, server_side=True)
threading.Thread(target=secure.serve_forever, daemon=True).start()
open(WORK + "/site.ready", "w").close()
plain.serve_forever()
