package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestKernelPayloadCarriesActiveShareSubscriptions(t *testing.T) {
	r, err := repo.Open(t.TempDir() + "/subscription.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	h := New(r, "unit-test-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`INSERT INTO node(name, secret, server_ip, port, created_time, status) VALUES('n', 's', '203.0.113.1', '1000-2000', ?, 1)`, now).Error; err != nil {
		t.Fatal(err)
	}
	nodeID := mustLastInsertID(t, r, "node")
	if err := r.DB().Exec(`INSERT INTO node_kernel(node_id, kernel_type, version, status, created_time, updated_time) VALUES(?, 'xray', 'latest', 'pending', ?, ?)`, nodeID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	kernelID := mustLastInsertID(t, r, "kernel")
	if err := r.DB().Exec(`INSERT INTO node_inbound(kernel_id, node_id, name, protocol, listen, port, settings, created_time, updated_time)
		VALUES(?, ?, 'a', 'trojan', '::', 443, '{"password":"owner-pw"}', ?, ?)`, kernelID, nodeID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	// "All inbounds" also covers inbounds added later.
	s := &model.Subscription{Name: "x", Kind: model.SubscriptionShare, Token: strings.Repeat("a", 32), UUID: "u",
		Password: "user-pw", SourceAll: 1, Enabled: 1, CreatedTime: now, UpdatedTime: now}
	if err := r.SaveSubscription(s); err != nil {
		t.Fatal(err)
	}
	own := &model.Subscription{Name: "mine", Kind: model.SubscriptionOwn, Token: strings.Repeat("b", 32),
		Password: "own-should-not-appear", SourceAll: 1, Enabled: 1, CreatedTime: now, UpdatedTime: now}
	if err := r.SaveSubscription(own); err != nil {
		t.Fatal(err)
	}

	config := func() string {
		k, _ := r.GetNodeKernel(kernelID)
		payload, _, err := h.buildKernelApplyPayload(k)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(payload["files"])
		return string(b)
	}
	if c := config(); !strings.Contains(c, "owner-pw") || !strings.Contains(c, "user-pw") || strings.Contains(c, "own-should-not-appear") {
		t.Fatalf("active share subscription must be in the config, own ones not: %s", c)
	}
	s.ExpireTime = now - 1000
	_ = r.SaveSubscription(s)
	if c := config(); strings.Contains(c, "user-pw") || !strings.Contains(c, "owner-pw") {
		t.Fatalf("expired share must be removed, owner kept: %s", c)
	}
}
