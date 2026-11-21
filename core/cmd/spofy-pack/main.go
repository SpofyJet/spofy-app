package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/pack"
)

func main() {
	stub := flag.String("stub", "", "the setup program without a payload")
	bundle := flag.String("bundle", "", "the folder that is installed")
	version := flag.String("version", "", "the version shown in the system")
	out := flag.String("out", "", "the installer to write")
	flag.Parse()
	if *stub == "" || *bundle == "" || *version == "" || *out == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := write(*stub, *bundle, *version, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type process struct {
	io.WriteCloser
	cmd *exec.Cmd
}

func (p *process) Close() error {
	return errors.Join(p.WriteCloser.Close(), p.cmd.Wait())
}

func xz(w io.Writer) (io.WriteCloser, error) {
	cmd := exec.Command("xz", "--threads=1", "--format=xz", "--check=crc64", "--lzma2=preset=9e,dict=64MiB", "--stdout")
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &process{in, cmd}, nil
}

func write(stub, bundle, version, out string) error {
	tool, err := exec.Command("xz", "--version").Output()
	if err != nil {
		return fmt.Errorf("XZ Utils are needed to pack: %w", err)
	}
	began := time.Now()
	in, err := os.Open(stub)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(out), ".pack-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	t, err := pack.Write(tmp, in, bundle, version, xz)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		return err
	}
	fmt.Printf("%s: %.2f MB (setup %.2f MB, payload %.2f MB of %.2f MB, %.1f%%) in %.1f s with %s\n",
		filepath.Base(out), float64(t.Offset+t.Size)/1e6, float64(t.Offset)/1e6, float64(t.Size)/1e6, float64(t.Unpacked)/1e6,
		100*float64(t.Size)/float64(t.Unpacked), time.Since(began).Seconds(), strings.SplitN(strings.TrimSpace(string(tool)), "\n", 2)[0])
	return nil
}
