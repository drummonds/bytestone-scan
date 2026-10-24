package bag

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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

// Deposit writes a photo's deposit to w: a tar with two top-level
// directories, bag/, the bag at bagDir relative to itself, and
// renditions/<recipe>/, the renditions at renditionsDir (the front's, the
// back's under back/). How the archive service takes a photo.
func Deposit(w io.Writer, bagDir, renditionsDir string, recipe int) error {
	tw := tar.NewWriter(w)
	if err := add(tw, bagDir, "bag/"); err != nil {
		return err
	}
	if err := add(tw, renditionsDir, "renditions/"+strconv.Itoa(recipe)+"/"); err != nil {
		return fmt.Errorf("renditions: %w", err)
	}
	return tw.Close()
}

// add writes every file under dir to the tar, its path relative to dir
// under prefix.
func add(tw *tar.Writer, dir, prefix string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
		if err := tw.WriteHeader(&tar.Header{Name: prefix + filepath.ToSlash(rel), Mode: 0o644, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}); err != nil {
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
}
