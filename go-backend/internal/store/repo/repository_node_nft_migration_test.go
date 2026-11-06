package repo

import (
	"path/filepath"
	"testing"

	"go-backend/internal/store/model"
)

func TestOpenBackfillsSQLiteNodeNftError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-node-nft.db")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	node := model.Node{Name: "existing-node", Secret: "existing-secret", ServerIP: "192.0.2.1", Port: "10000-20000", Status: 1}
	if err := r.db.Create(&node).Error; err != nil {
		_ = r.Close()
		t.Fatal(err)
	}
	if err := r.db.Exec("ALTER TABLE node DROP COLUMN nft_error").Error; err != nil {
		_ = r.Close()
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		r, err = Open(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", attempt, err)
		}
		func() {
			defer r.Close()
			if !r.db.Migrator().HasColumn(&model.Node{}, "NftError") {
				t.Fatal("existing SQLite node table is missing nft_error after startup")
			}
			got, err := r.GetNodeByID(node.ID)
			if err != nil || got == nil {
				t.Fatalf("read existing node: %+v %v", got, err)
			}
			wantError := ""
			if attempt > 0 {
				wantError = "nft: Operation not permitted"
			}
			if got.Name != node.Name || got.Secret != node.Secret || got.ServerIP != node.ServerIP || got.NftError != wantError {
				t.Fatalf("node data changed during migration: %+v", got)
			}
			if changed, err := r.SetNodeNftError(node.ID, "nft: Operation not permitted"); err != nil || changed != (attempt == 0) {
				t.Fatalf("save nft status after reopen %d: changed=%v err=%v", attempt, changed, err)
			}
		}()
	}
}
