import hashlib
import re
import ssl
import sys


def prints(path):
    return {hashlib.sha256(ssl.PEM_cert_to_DER_cert(c)).hexdigest() for c in re.findall(r"-----BEGIN CERTIFICATE-----.+?-----END CERTIFICATE-----", open(path).read(), re.S)}


ours, system = prints(sys.argv[1]), prints(sys.argv[2])
foreign = ours - system
print("FAIL: %d bundled roots, %d of them are not in the trust store of this system" % (len(ours), len(foreign)) if foreign or not ours else "ROOTS OK %d" % len(ours))
sys.exit(1 if foreign or not ours else 0)
