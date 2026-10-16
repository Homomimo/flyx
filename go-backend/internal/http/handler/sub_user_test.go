package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestKernelPayloadCarriesActiveSubUsers(t *testing.T) {
	r, err := repo.Open(t.TempDir() + "/subuser.db")
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
	inboundID := mustLastInsertID(t, r, "inbound")
	u := &model.SubUser{Name: "x", Token: strings.Repeat("a", 32), UUID: "u", Password: "user-pw",
		InboundIDs: formatIDList([]int64{inboundID}), Enabled: 1, CreatedTime: now, UpdatedTime: now}
	if err := r.SaveSubUser(u); err != nil {
		t.Fatal(err)
	}

	config := func() string {
		k, _ := r.GetNodeKernel(kernelID)
		payload, _, err := h.buildKernelApplyPayload(k)
		if err != nil {
			t.Fatal(err)
		}
		return mustJSONString(t, payload["files"])
	}
	if c := config(); !strings.Contains(c, "owner-pw") || !strings.Contains(c, "user-pw") {
		t.Fatalf("active user must be in the config: %s", c)
	}
	u.ExpireTime = now - 1000
	_ = r.SaveSubUser(u)
	if c := config(); strings.Contains(c, "user-pw") || !strings.Contains(c, "owner-pw") {
		t.Fatalf("expired user must be removed, owner kept: %s", c)
	}
}

func mustJSONString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
