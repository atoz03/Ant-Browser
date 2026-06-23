package browser

import (
	"ant-chrome/backend/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteKeepsProfileUserDataDirDuringTrashRetention(t *testing.T) {
	appRoot := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = "data"
	mgr := NewManager(cfg, appRoot)
	profile := &Profile{ProfileId: "profile-1", UserDataDir: "profile-1"}
	mgr.Profiles[profile.ProfileId] = profile

	profileDir := filepath.Join(appRoot, "data", "profile-1")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := mgr.Delete(profile.ProfileId); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := os.Stat(profileDir); err != nil {
		t.Fatalf("expected profile dir kept during trash retention, stat err=%v", err)
	}
}

func TestDeleteKeepsUserDataRootWhenProfileDirIsRoot(t *testing.T) {
	appRoot := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = "data"
	mgr := NewManager(cfg, appRoot)
	profile := &Profile{ProfileId: "profile-root", UserDataDir: ""}
	mgr.Profiles[profile.ProfileId] = profile

	rootDir := filepath.Join(appRoot, "data")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	profile.UserDataDir = rootDir

	if err := mgr.Delete(profile.ProfileId); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := os.Stat(rootDir); err != nil {
		t.Fatalf("expected data root kept, stat err=%v", err)
	}
}

func TestCleanupExpiredTrashRemovesProfileDataAndSnapshots(t *testing.T) {
	appRoot := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = "data"
	mgr := NewManager(cfg, appRoot)
	profile := &Profile{
		ProfileId:   "profile-expired",
		UserDataDir: "profile-expired",
		DeletedAt:   time.Now().Add(-profileTrashRetention - time.Hour).Format(time.RFC3339),
	}

	profileDir := filepath.Join(appRoot, "data", "profile-expired")
	snapshotDir := filepath.Join(appRoot, "data", "snapshots", "profile-expired")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll profile dir failed: %v", err)
	}
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("MkdirAll snapshot dir failed: %v", err)
	}

	mgr.ProfileDAO = &memoryExpiredProfileDAO{expired: []*Profile{profile}}
	if err := mgr.CleanupExpiredTrash(); err != nil {
		t.Fatalf("CleanupExpiredTrash failed: %v", err)
	}
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatalf("expected profile dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
		t.Fatalf("expected snapshot dir removed, stat err=%v", err)
	}
}

type memoryExpiredProfileDAO struct {
	expired []*Profile
}

func (d *memoryExpiredProfileDAO) List() ([]*Profile, error)                           { return nil, nil }
func (d *memoryExpiredProfileDAO) ListDeleted() ([]*Profile, error)                    { return nil, nil }
func (d *memoryExpiredProfileDAO) GetById(profileId string) (*Profile, error)          { return nil, nil }
func (d *memoryExpiredProfileDAO) Upsert(profile *Profile) error                       { return nil }
func (d *memoryExpiredProfileDAO) Delete(profileId string) error                       { return nil }
func (d *memoryExpiredProfileDAO) SoftDelete(profileId string, deletedAt string) error { return nil }
func (d *memoryExpiredProfileDAO) Restore(profileId string) error                      { return nil }
func (d *memoryExpiredProfileDAO) ListExpiredDeleted(expiredBefore string) ([]*Profile, error) {
	return d.expired, nil
}
