import hashlib
import io
import os
import sys
import urllib.request
import zipfile

URL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
SHA256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"

arch, out = sys.argv[1], sys.argv[2]
data = urllib.request.urlopen(urllib.request.Request(URL, headers={"User-Agent": "curl"}), timeout=120).read()
if hashlib.sha256(data).hexdigest() != SHA256:
    sys.exit("FAIL: the Wintun archive is not the pinned one")
library = zipfile.ZipFile(io.BytesIO(data)).read("wintun/bin/%s/wintun.dll" % arch)
with open(os.path.join(out, "wintun.dll"), "wb") as f:
    f.write(library)
print("WINTUN OK %d bytes" % len(library))
