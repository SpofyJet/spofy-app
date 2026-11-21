package desk

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	Access = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	App    = "app.spofy"
)

func Root() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Spofy"), nil
}

func Secure(path string) error {
	if attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path)); err == nil {
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || !trusted(path) {
			if err := os.Rename(path, fmt.Sprintf("%s.untrusted-%d", path, time.Now().UnixNano())); err != nil {
				return err
			}
		}
	}
	sd, err := windows.SecurityDescriptorFromString(Access)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(windows.StringToUTF16Ptr(path), sa)
	if err == nil {
		return own(path)
	}
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func Forget(path string) error {
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || !trusted(path) {
		return fmt.Errorf("%s is not ours, left as it is", path)
	}
	return os.RemoveAll(path)
}

func own(path string) error {
	if trusted(path) {
		return nil
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err == nil {
		err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, admins, nil, nil, nil)
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

func trusted(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return false
	}
	return owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func Key(file string) ([]byte, error) {
	if blob, err := os.ReadFile(file); err == nil {
		if key, err := unprotect(blob); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	blob, err := protect(key)
	if err != nil {
		return nil, err
	}
	tmp := file + ".new"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return key, nil
}

func protect(b []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), nil
}

func unprotect(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), nil
}

func Identity() (hwid, release, model string) {
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		if id, _, err := k.GetStringValue("MachineGuid"); err == nil && id != "" {
			sum := sha256.Sum256([]byte(id + ":" + App))
			hwid = hex.EncodeToString(sum[:16])
		}
		k.Close()
	}
	v := windows.RtlGetVersion()
	release = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		maker, _, _ := k.GetStringValue("SystemManufacturer")
		product, _, _ := k.GetStringValue("SystemProductName")
		k.Close()
		model = strings.TrimSpace(strings.TrimSpace(maker) + " " + strings.TrimSpace(product))
	}
	if model == "" {
		model = "PC"
	}
	return hwid, release, model
}
