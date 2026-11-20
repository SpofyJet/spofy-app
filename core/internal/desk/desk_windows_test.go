package desk

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestKeyIsKeptSealedAndReplacedWhenItCannotBeRead(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.key")
	first, err := Key(file)
	if err != nil || len(first) != 32 {
		t.Fatalf("key: %d bytes, %v", len(first), err)
	}
	again, err := Key(file)
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("the key changed between starts: %v", err)
	}
	blob, _ := os.ReadFile(file)
	if bytes.Contains(blob, first) {
		t.Fatal("the key lies in the file as it is")
	}
	blob[len(blob)/2] ^= 0xff
	os.WriteFile(file, blob, 0o600)
	fresh, err := Key(file)
	if err != nil || len(fresh) != 32 || bytes.Equal(fresh, first) {
		t.Fatalf("a spoiled file did not give a new key: %v", err)
	}
	if kept, _ := Key(file); !bytes.Equal(kept, fresh) {
		t.Fatal("the new key was not kept")
	}
}

func dacl(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func onlySystemAndAdmins(t *testing.T, path string, own bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, _ := sd.Control()
	if own && control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("%s takes rights from its parent: %s", path, sd)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		t.Fatalf("no access list on %s", path)
	}
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			t.Fatalf("%s lets in %s: %s", path, sid, sd)
		}
	}
}

func TestFolderIsOpenOnlyToTheSystemAndAdministrators(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "Spofy")
	if err := Secure(path); err != nil {
		t.Fatal(err)
	}
	onlySystemAndAdmins(t, path, true)
	inner := filepath.Join(path, "state")
	if err := os.Mkdir(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	onlySystemAndAdmins(t, inner, false)
	open := filepath.Join(base, "Open")
	os.Mkdir(open, 0o755)
	if out, err := exec.Command("icacls", open, "/grant", "*S-1-5-32-545:(OI)(CI)F").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v %s", err, out)
	}
	if !strings.Contains(dacl(t, open), ";;;BU)") {
		t.Fatalf("the stand did not open the folder to users: %s", dacl(t, open))
	}
	if err := Secure(open); err != nil {
		t.Fatal(err)
	}
	onlySystemAndAdmins(t, open, true)
}

func TestALinkInPlaceOfTheFolderIsMovedAsideAndItsTargetIsLeftAlone(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "elsewhere")
	os.Mkdir(target, 0o755)
	os.WriteFile(filepath.Join(target, "keep.txt"), []byte("x"), 0o644)
	before := dacl(t, target)
	path := filepath.Join(base, "Spofy")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", path, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if err := Secure(path); err != nil {
		t.Fatal(err)
	}
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		t.Fatalf("the folder is still a link: %#x %v", attrs, err)
	}
	onlySystemAndAdmins(t, path, true)
	if after := dacl(t, target); after != before {
		t.Fatalf("the link's target changed its rights: %s -> %s", before, after)
	}
	if _, err := os.Stat(filepath.Join(target, "keep.txt")); err != nil {
		t.Fatal("the link's target lost its files")
	}
	moved, _ := filepath.Glob(path + ".untrusted-*")
	if len(moved) != 1 {
		t.Fatalf("the link was not moved aside: %v", moved)
	}
}

func TestIdentityIsStableAndSaysNothingRaw(t *testing.T) {
	hwid, release, model := Identity()
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(hwid) {
		t.Fatalf("device id %q", hwid)
	}
	if again, _, _ := Identity(); again != hwid {
		t.Fatal("the device id changed")
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(release) || model == "" {
		t.Fatalf("release %q, model %q", release, model)
	}
}
