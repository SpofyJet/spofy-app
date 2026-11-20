package engine

import (
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func linked(t *testing.T, file string) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		out[path] = true
	}
	return out
}

func TestPartsFollowTheForkAndLeaveOutOnlyWhatIsNamed(t *testing.T) {
	const fork = "github.com/xtls/xray-core/"
	dir, err := exec.Command("go", "list", "-f", "{{.Dir}}", fork+"main/distro/all").Output()
	if err != nil {
		t.Fatal(err)
	}
	theirs := linked(t, filepath.Join(strings.TrimSpace(string(dir)), "all.go"))
	ours := linked(t, "parts.go")
	left := map[string]bool{}
	for _, name := range []string{"main/commands/all", "main/confloader/external", "main/json", "main/toml", "main/yaml"} {
		left[fork+name] = true
	}
	if len(theirs) < 40 {
		t.Fatalf("the fork's full set has only %d parts", len(theirs))
	}
	for path := range theirs {
		if ours[path] == left[path] {
			t.Errorf("%s is in the fork's full set: here it must be either linked or left out on purpose", path)
		}
	}
	for path := range ours {
		if !theirs[path] {
			t.Errorf("%s is linked here and is not in the fork's full set", path)
		}
	}
	for path := range left {
		if !theirs[path] {
			t.Errorf("%s is left out here and the fork no longer has it", path)
		}
	}
	deps, err := exec.Command("go", "list", "-deps", "github.com/SpofyJet/spofy-app/core/cmd/spofy-core").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"github.com/pelletier/go-toml", "github.com/ghodss/yaml", fork + "main/commands", fork + "infra/conf/serial"} {
		if strings.Contains(string(deps), gone) {
			t.Errorf("%s is in the build again", gone)
		}
	}
}
