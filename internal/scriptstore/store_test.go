package scriptstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func storeFixture(t *testing.T) *Store {
	t.Helper()
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestCreateSaveConflictAndPersistence(t *testing.T) {
	s := storeFixture(t)
	doc, e := s.Create("中文脚本.py", "print('你好')\n")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Create("中文脚本.py", "overwrite"); e == nil {
		t.Fatal("overwrote file")
	}
	saved, e := s.Save(doc.Path, "print('已保存')\n", doc.Version)
	if e != nil || saved.Version == doc.Version {
		t.Fatalf("save: %+v %v", saved, e)
	}
	b, _ := os.ReadFile(doc.Path)
	if string(b) != saved.Content {
		t.Fatal("save was not persisted")
	}
	if _, e = s.Save(doc.Path, "stale", doc.Version); e == nil {
		t.Fatal("accepted stale version")
	}
	if e = os.WriteFile(doc.Path, []byte("external"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(doc.Path, "overwrite", saved.Version); e == nil {
		t.Fatal("overwrote external changes")
	}
	b, _ = os.ReadFile(doc.Path)
	if string(b) != "external" {
		t.Fatal("changed disk on conflict")
	}
	list, e := s.List()
	if e != nil || len(list.Files) != 1 {
		t.Fatalf("list %+v %v", list, e)
	}
}
func TestPathsBoundedUntilNativeOpen(t *testing.T) {
	s := storeFixture(t)
	path := filepath.Join(t.TempDir(), "external.py")
	os.WriteFile(path, []byte("print(1)"), 0600)
	if _, e := s.Read(path); e == nil {
		t.Fatal("read unapproved path")
	}
	if _, e := s.Save(path, "new", "version"); e == nil {
		t.Fatal("saved unapproved path")
	}
	for _, name := range []string{"../escape.py", `..\escape.py`, "file.txt", "foo/bar.py", "x\n.py", "a:stream.py", ""} {
		if _, e := s.Create(name, ""); e == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	doc, e := s.Open(path)
	if e != nil || !doc.External {
		t.Fatalf("open %+v %v", doc, e)
	}
	if _, e = s.Save(path, "print(2)", doc.Version); e != nil {
		t.Fatal(e)
	}
	restored, e := New(filepath.Dir(s.root))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = restored.Read(path); e != nil {
		t.Fatal("lost authorized external path", e)
	}
	if list, e := restored.List(); e != nil || len(list.Files) != 1 || !list.Files[0].External {
		t.Fatalf("list %+v %v", list, e)
	}
}
func TestSizeEncodingAndSymlinkGuards(t *testing.T) {
	s := storeFixture(t)
	for _, content := range []string{strings.Repeat("x", MaxSize+1), string([]byte{0xff}), "a\x00b"} {
		if _, e := s.Create("invalid.py", content); e == nil {
			t.Fatal("accepted invalid content")
		}
	}
	big := filepath.Join(s.root, "big.py")
	os.WriteFile(big, make([]byte, MaxSize+1), 0600)
	if _, e := s.Read(big); e == nil {
		t.Fatal("read oversized file")
	}
	d, e := s.Create("target.py", "print(1)")
	if e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(s.root, "link.py")
	if e = os.Symlink(d.Path, link); e == nil {
		if _, e = s.Read(link); e == nil {
			t.Fatal("followed symlink")
		}
	}
}
func TestHelpIsRealMarkdownOutsideScriptList(t *testing.T) {
	s := storeFixture(t)
	path, e := s.Guide([]byte("# 开发指南\n"))
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Ext(path) != ".md" {
		t.Fatal(path)
	}
	b, e := os.ReadFile(path)
	if e != nil || string(b) != "# 开发指南\n" {
		t.Fatalf("guide %q %v", b, e)
	}
	if _, e = s.Read(path); e == nil {
		t.Fatal("guide allowed as Python")
	}
	if list, e := s.List(); e != nil || len(list.Files) != 0 {
		t.Fatal("guide leaked into script list")
	}
}

func TestTrustPersistsForExactFileVersionAndCanBeRevoked(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := store.Create("trusted.py", "print('trusted')\n")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Trusted {
		t.Fatal("new file trusted without consent")
	}
	doc, err = store.Trust(doc.Path, doc.Version, true)
	if err != nil || !doc.Trusted {
		t.Fatal("trust failed", err)
	}
	store, err = New(dir)
	if err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(doc.Path)
	if err != nil || !read.Trusted {
		t.Fatal("trust not restored after restart", err)
	}
	other, err := store.Create("other.py", doc.Content)
	if err != nil || other.Trusted {
		t.Fatal("another path inherited trust", err)
	}
	changed, err := store.Save(doc.Path, "print('changed')\n", doc.Version)
	if err != nil || changed.Trusted {
		t.Fatal("changed file inherited trust", err)
	}
	if _, err := store.Trust(doc.Path, doc.Version, true); err == nil {
		t.Fatal("stale revision trusted")
	}
	changed, err = store.Trust(changed.Path, changed.Version, true)
	if err != nil || !changed.Trusted {
		t.Fatal("new version not trusted", err)
	}
	changed, err = store.Trust(changed.Path, changed.Version, false)
	if err != nil || changed.Trusted {
		t.Fatal("revocation failed", err)
	}
	store, err = New(dir)
	if err != nil {
		t.Fatal(err)
	}
	read, err = store.Read(doc.Path)
	if err != nil || read.Trusted {
		t.Fatal("revocation not persisted", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", ".trusted.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err = New(dir)
	if err != nil {
		t.Fatal("bad trust metadata must not hide scripts", err)
	}
	read, err = store.Read(doc.Path)
	if err != nil || read.Trusted {
		t.Fatal("corrupt trust metadata did not fail closed", err)
	}
}
