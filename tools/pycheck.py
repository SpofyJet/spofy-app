import ast, builtins, symtable, sys

bad = []
for path in sys.argv[1:]:
    source = open(path).read()
    tree = ast.parse(source, path)
    table = symtable.symtable(source, path, "exec")
    known = set(dir(builtins)) | {"__file__"} | {s.get_name() for s in table.get_symbols() if s.is_assigned() or s.is_imported() or s.is_namespace()}
    kept = {t.id for n in tree.body if isinstance(n, ast.Assign) and isinstance(n.value, (ast.List, ast.Dict, ast.Set)) for t in n.targets if isinstance(t, ast.Name)}
    bad += ["%s: unknown name %s" % (path, s.get_name()) for s in table.get_symbols() if s.get_name() not in known]

    def walk(scope):
        for inner in scope.get_children():
            for s in inner.get_symbols():
                if s.is_global() and s.get_name() not in known:
                    bad.append("%s: %s uses the unknown name %s" % (path, inner.get_name(), s.get_name()))
                if s.is_local() and s.is_assigned() and s.get_name() in kept:
                    bad.append("%s: %s has its own %s and hides the list of the same name" % (path, inner.get_name(), s.get_name()))
            walk(inner)
    walk(table)
print("\n".join(bad) if bad else "PYTHON OK")
sys.exit(1 if bad else 0)
