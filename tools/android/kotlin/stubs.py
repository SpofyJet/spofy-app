import glob
import os
import re
import sys

res, out = sys.argv[1], sys.argv[2]
groups = {}
for path in sorted(glob.glob(res + "/*/*")):
    folder = os.path.basename(os.path.dirname(path)).split("-")[0]
    if folder == "values":
        for kind, name in re.findall(r'<(string|style|color|dimen|plurals)\s+name="([^"]+)"', open(path).read()):
            groups.setdefault(kind, set()).add(name.replace(".", "_"))
    else:
        groups.setdefault(folder, set()).add(os.path.splitext(os.path.basename(path))[0])
lines = ["package app.spofy", "", "object BuildConfig {", "    @JvmField", "    val DEBUG = System.nanoTime() > 0", "}", "", "object R {"]
n = 0
for kind in sorted(groups):
    lines.append("    object %s {" % kind)
    for name in sorted(groups[kind]):
        n += 1
        lines.append("        const val %s = %d" % (name, n))
    lines.append("    }")
lines.append("}")
open(out, "w").write("\n".join(lines) + "\n")
