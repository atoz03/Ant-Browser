package database

import (
	"path/filepath"
	"testing"
)

func TestMigrateVersion12DatabaseAddsExtensionScopeTables(t *testing.T) {
	db, err := NewDB(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.conn.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		desc TEXT NOT NULL DEFAULT '',
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.conn.Exec(`INSERT INTO schema_migrations (version, desc) VALUES (12, '旧版本')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"browser_extension_scope_settings", "browser_extension_scope_profiles"} {
		var count int
		if err := db.conn.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("迁移后缺少数据表: %s", table)
		}
	}
}
