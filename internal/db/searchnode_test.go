package db

import (
	"path/filepath"
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSearchNodeTestDB(t *testing.T) {
	t.Helper()
	conf.Conf = &conf.Config{Database: conf.Database{Type: "sqlite3"}}
	d, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	Init(d)
	if err := d.AutoMigrate(&model.SearchNode{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := d.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// Regression: DeleteSearchNodesByParent must remove the directory's OWN node
// (parent=Dir(path), name=Base(path)) plus its subtree. The old code used
// stdpath.Split, whose dir keeps a trailing slash and never matched the stored
// parent, so every per-path update duplicated the directory's own node.
func TestDeleteSearchNodesByParentRemovesSelfAndSubtree(t *testing.T) {
	setupSearchNodeTestDB(t)
	nodes := []model.SearchNode{
		{Parent: "/a/b", Name: "c", IsDir: true},
		{Parent: "/a/b/c", Name: "file.txt"},
		{Parent: "/a/b/c", Name: "sub", IsDir: true},
		{Parent: "/a/b/c/sub", Name: "deep.txt"},
		{Parent: "/a/b", Name: "other", IsDir: true},
	}
	if err := BatchCreateSearchNodes(&nodes); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := DeleteSearchNodesByParent("/a/b/c"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var remaining []model.SearchNode
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Parent != "/a/b" || remaining[0].Name != "other" {
		t.Fatalf("expected only sibling /a/b:other to remain, got %+v", remaining)
	}
}

// Repeated delete+reindex (what the admin "update" button does) must not
// accumulate duplicate copies of the directory's own node.
func TestUpdateDoesNotDuplicateSelfNode(t *testing.T) {
	setupSearchNodeTestDB(t)
	reindex := func() {
		if err := DeleteSearchNodesByParent("/a/b/c"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		nodes := []model.SearchNode{
			{Parent: "/a/b", Name: "c", IsDir: true},
			{Parent: "/a/b/c", Name: "file.txt"},
		}
		if err := BatchCreateSearchNodes(&nodes); err != nil {
			t.Fatalf("reindex: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		reindex()
	}
	var n int64
	db.Model(&model.SearchNode{}).Where("parent = ? AND name = ?", "/a/b", "c").Count(&n)
	if n != 1 {
		t.Fatalf("self node duplicated: got %d copies, want 1", n)
	}
}
