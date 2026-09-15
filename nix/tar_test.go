package nix

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlewo/nix2container/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
)

func TestTar(t *testing.T) {
	path := types.Path{
		Path: "../data/tar-directory",
	}
	digest, size, err := TarPathsSum(types.Paths{path})
	if err != nil {
		t.Fatalf("%v", err)
	}
	expectedDigest := "sha256:1ea63d00b937dc24c711265b80444cc9e7e63751fb7f349b160be61d31381983"
	assert.Equal(t, expectedDigest, digest.String())

	expectedSize := int64(4096)
	assert.Equal(t, expectedSize, size)
	if size != expectedSize {
		t.Errorf("Size is %d while it should be %d", size, expectedSize)
	}
}

func TestRemoveNixCaseHackSuffix(t *testing.T) {
	ret := removeNixCaseHackSuffix("filename~nix~case~hack~1")
	expected := "filename"
	if ret != expected {
		t.Errorf("%s should be %s", ret, expected)
	}
	ret = removeNixCaseHackSuffix("/path~nix~case~hack~1/filename")
	expected = "/path/filename"
	if ret != expected {
		t.Errorf("%s should be %s", ret, expected)
	}
	ret = removeNixCaseHackSuffix("filename~nix~")
	expected = "filename~nix~"
	if ret != expected {
		t.Errorf("%s should be %s", ret, expected)
	}
}

// makeTree writes a tree with directories, small and large files and a
// symlink, and returns its root.
func makeTree(t testing.TB, dirs, filesPerDir int) string {
	t.Helper()
	root := t.TempDir()
	for d := 0; d < dirs; d++ {
		dir := filepath.Join(root, fmt.Sprintf("dir%03d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f := 0; f < filesPerDir; f++ {
			// Sizes from a few bytes to more than the 32 KiB copy buffer.
			data := bytes.Repeat([]byte{byte('a' + f%26)}, 1+(f*7919)%70000)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%03d", f)), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Symlink("dir000/file000", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	return root
}

// The file written by TarPathsWrite must hold the same bytes as the
// stream that TarPathsSum hashes.
func TestTarPathsWriteMatchesSum(t *testing.T) {
	paths := types.Paths{{Path: makeTree(t, 5, 20)}}
	sum, size, err := TarPathsSum(paths)
	if err != nil {
		t.Fatal(err)
	}
	path, written, writtenSize, err := TarPathsWrite(paths, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, sum, written)
	assert.Equal(t, size, writtenSize)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, size, int64(len(data)))
	assert.Equal(t, sum, digest.FromBytes(data))
}

func BenchmarkTarPathsSum(b *testing.B) {
	paths := types.Paths{{Path: makeTree(b, 50, 100)}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := TarPathsSum(paths); err != nil {
			b.Fatal(err)
		}
	}
}

// Invalid orMode values must fail loudly at tar time, not be silently
// misparsed: Sscanf-style laxity here corrupts header modes (a negative
// mode even flips archive/tar to GNU base-256 encoding).
func TestTarPermsOrModeInvalid(t *testing.T) {
	for _, orMode := range []string{"0o311", "-0200", "40755", "8"} {
		t.Run(orMode, func(t *testing.T) {
			paths := types.Paths{{
				Path: "../data/layer1/file1",
				Options: &types.PathOptions{
					Perms: []types.Perm{{Regex: ".*", OrMode: orMode}},
				},
			}}
			r := TarPaths(paths)
			defer r.Close() // nolint: errcheck
			tr := tar.NewReader(r)
			var err error
			for err == nil {
				_, err = tr.Next()
			}
			assert.ErrorContains(t, err, "invalid orMode")
		})
	}
}

func TestTarPathsExcludes(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"share/doc/a", "share/doc/b", "share/docs", "bin"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, d, "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := types.Paths{{Path: root, Options: &types.PathOptions{Excludes: []string{"share/doc", "bin/f"}}}}
	r := TarPaths(paths)
	defer r.Close() // nolint: errcheck
	tr := tar.NewReader(r)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, strings.TrimPrefix(hdr.Name, root))
	}
	joined := strings.Join(names, "\n")
	assert.NotContains(t, joined, "/share/doc/")
	assert.NotContains(t, joined, "/share/doc\n")
	assert.NotContains(t, joined, "/bin/f")
	assert.Contains(t, joined, "/share/docs/f")
	assert.Contains(t, joined, "/bin\n")
}
