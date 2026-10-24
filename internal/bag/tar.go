package bag

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Manifest reads the bag's payload manifest: path within the bag → sha256.
func Manifest(dir string) (map[string]string, error) {
	f, err := os.Open(filepath.Join(dir, "manifest-sha256.txt"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s: a manifest line is not <sum> <path>", dir)
		}
		m[fields[1]] = strings.ToLower(fields[0])
	}
	return m, sc.Err()
}

// Tar writes the bag at dir to w as a tar of its directory, every path
// relative to the bag: how the archive service takes a deposit.
func Tar(dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}
