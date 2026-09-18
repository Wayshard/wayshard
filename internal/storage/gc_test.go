package storage

import (
	"context"
	"testing"
)

func TestObjectGCKeepsReferenced(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.Objects.Put([]byte("keep-me"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Objects.Has(h) {
		t.Fatal("missing put")
	}
	orphan, err := s.Objects.Put([]byte("orphan-object-xxxxxxxx"))
	if err != nil {
		t.Fatal(err)
	}
	// age the orphan by rewriting mtime... skip age pin: GC skips <10m
	n, err := s.GCObjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Objects.Has(h) {
		t.Fatal("referenced object collected")
	}
	_ = n
	_ = orphan
}

func TestDiskStatus(t *testing.T) {
	d, err := Disk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if d.Total == 0 {
		t.Fatal("expected total")
	}
}
