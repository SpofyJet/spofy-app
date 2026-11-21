package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/update"
)

var errUsage = errors.New("usage: spofy-release keygen|manifest|sign|verify [flags]")

func main() {
	err := run(os.Args[1:], os.Stdout)
	switch {
	case errors.Is(err, errUsage):
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	commands := map[string]func([]string, io.Writer) error{
		"keygen":   keygen,
		"manifest": manifest,
		"sign":     sign,
		"verify":   verify,
	}
	if len(args) == 0 || commands[args[0]] == nil {
		return errUsage
	}
	return commands[args[0]](args[1:], out)
}

type list []string

func (l *list) String() string {
	return strings.Join(*l, " ")
}

func (l *list) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func parse(fs *flag.FlagSet, args []string, required ...string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%w: unexpected %q", errUsage, fs.Arg(0))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, name := range required {
		if !set[name] {
			return fmt.Errorf("%w: %s needs -%s", errUsage, fs.Name(), name)
		}
	}
	return nil
}

func keygen(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	path := fs.String("out", "", "file for the new private key")
	if err := parse(fs, args, "out"); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed()))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(*path)
		return err
	}
	_, err = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(pub))
	return err
}

func manifest(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
	version := fs.String("version", "", "version shown to people")
	build := fs.Int("build", 0, "build number, the only ordering key")
	notesFile := fs.String("notes-file", "", "file with what is new")
	minBuild := fs.Int("min", 0, "builds below it must update")
	rollout := fs.Int("rollout", 0, "percent of installs offered the update")
	var bases, files list
	fs.Var(&bases, "url-base", "address the files are served under, repeatable")
	fs.Var(&files, "file", "platform=path, repeatable")
	path := fs.String("out", "", "manifest to write")
	if err := parse(fs, args, "version", "build", "notes-file", "rollout", "url-base", "file", "out"); err != nil {
		return err
	}
	notes, err := os.ReadFile(*notesFile)
	if err != nil {
		return err
	}
	m := update.Manifest{
		Version:  *version,
		Build:    *build,
		Released: time.Now().UTC().Truncate(time.Second),
		Notes:    strings.TrimSpace(string(notes)),
		Min:      *minBuild,
		Rollout:  *rollout,
		Files:    map[string]update.File{},
	}
	byName := map[string]string{}
	for _, spec := range files {
		platform, file, ok := strings.Cut(spec, "=")
		if !ok || !slices.Contains(update.Platforms(), platform) {
			return fmt.Errorf("-file %q: want platform=path, platform one of %s", spec, strings.Join(update.Platforms(), ", "))
		}
		if _, dup := m.Files[platform]; dup {
			return fmt.Errorf("-file %s is given twice", platform)
		}
		f, err := describe(file, bases)
		if err != nil {
			return err
		}
		if sum, seen := byName[f.Name]; seen && sum != f.SHA256 {
			return fmt.Errorf("two different files are named %s and would share one address", f.Name)
		}
		byName[f.Name] = f.SHA256
		m.Files[platform] = f
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	if _, err := update.ParseManifest(b.Bytes()); err != nil {
		return err
	}
	if err := os.WriteFile(*path, b.Bytes(), 0o644); err != nil {
		return err
	}
	return summary(out, &m)
}

func describe(path string, bases []string) (update.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return update.File{}, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return update.File{}, err
	}
	file := update.File{Name: filepath.Base(path), Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}
	for _, base := range bases {
		u, err := url.JoinPath(base, file.Name)
		if err != nil {
			return update.File{}, fmt.Errorf("-url-base %q: %w", base, err)
		}
		file.URLs = append(file.URLs, u)
	}
	return file, nil
}

func sign(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	keyFile := fs.String("key", "", "private key from keygen")
	manifestFile := fs.String("manifest", "", "manifest to sign")
	path := fs.String("out", "", "update file to write")
	if err := parse(fs, args, "key", "manifest", "out"); err != nil {
		return err
	}
	raw, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return fmt.Errorf("%s is not a key from keygen", *keyFile)
	}
	key := ed25519.NewKeyFromSeed(seed)
	pub := key.Public().(ed25519.PublicKey)
	b, err := os.ReadFile(*manifestFile)
	if err != nil {
		return err
	}
	env, err := update.Seal(b, key)
	if err != nil {
		return err
	}
	m, err := update.Open(env, []ed25519.PublicKey{pub})
	if err != nil {
		return err
	}
	if err := os.WriteFile(*path, env, 0o644); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "signed by %s\n", base64.StdEncoding.EncodeToString(pub)); err != nil {
		return err
	}
	return summary(out, m)
}

func verify(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pub := fs.String("pub", "", "public key from keygen, base64")
	in := fs.String("in", "", "update file to check")
	if err := parse(fs, args, "pub", "in"); err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(*pub)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("-pub is not a public key from keygen")
	}
	b, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	m, err := update.Open(b, []ed25519.PublicKey{key})
	if err != nil {
		return err
	}
	return summary(out, m)
}

func summary(w io.Writer, m *update.Manifest) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s build %d, released %s, min %d, rollout %d%%\n", m.Version, m.Build, m.Released.Format(time.RFC3339), m.Min, m.Rollout)
	for _, p := range slices.Sorted(maps.Keys(m.Files)) {
		f := m.Files[p]
		fmt.Fprintf(&b, "%s: %s, %d bytes, sha256 %s\n", p, f.Name, f.Size, f.SHA256)
		for _, u := range f.URLs {
			fmt.Fprintf(&b, "  %s\n", u)
		}
	}
	fmt.Fprintf(&b, "notes:\n%s\n", m.Notes)
	_, err := io.WriteString(w, b.String())
	return err
}
