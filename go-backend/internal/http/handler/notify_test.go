package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func setupNotifyHandler(t *testing.T) (*Handler, *repo.Repository, func() []notifyMessage) {
	t.Helper()
	r, err := repo.Open(t.TempDir() + "/notify.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	var mu sync.Mutex
	var got []notifyMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var m notifyMessage
		_ = json.NewDecoder(req.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	now := time.Now().UnixMilli()
	for k, v := range map[string]string{"notify_enabled": "true", "notify_webhook_url": srv.URL} {
		if err := r.DB().Exec(`INSERT INTO vite_config(name, value, time) VALUES(?, ?, ?)`, k, v, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	h := New(r, "unit-test-secret")
	return h, r, func() []notifyMessage {
		time.Sleep(150 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]notifyMessage(nil), got...)
	}
}

func TestNodeOfflineNotifyIsDebounced(t *testing.T) {
	old := nodeOfflineGrace
	nodeOfflineGrace = 50 * time.Millisecond
	t.Cleanup(func() { nodeOfflineGrace = old })
	h, _, sent := setupNotifyHandler(t)

	// A quick reconnect within the grace period stays silent.
	h.notifyNodeOffline(7)
	h.notifyNodeOnline(7)
	if msgs := sent(); len(msgs) != 0 {
		t.Fatalf("short reconnect must not notify: %v", msgs)
	}

	h.notifyNodeOffline(7)
	if msgs := sent(); len(msgs) != 1 || msgs[0].Title != "节点掉线" || msgs[0].Event != notifyEventNodeOffline {
		t.Fatalf("offline alert: %v", msgs)
	}
	h.notifyNodeOffline(7)
	if msgs := sent(); len(msgs) != 1 {
		t.Fatalf("an ongoing outage must alert once: %v", msgs)
	}
	h.notifyNodeOnline(7)
	if msgs := sent(); len(msgs) != 2 || msgs[1].Title != "节点恢复在线" {
		t.Fatalf("recovery alert: %v", msgs)
	}
}

func TestNodeExpiryNotifiesOncePerDate(t *testing.T) {
	h, r, sent := setupNotifyHandler(t)
	now := time.Now()
	insert := func(name string, expiry time.Time, dismissed int) {
		t.Helper()
		if err := r.DB().Exec(`INSERT INTO node(name, secret, server_ip, port, created_time, status, expiry_time, expiry_reminder_dismissed)
			VALUES(?, ?, '203.0.113.1', '1000-2000', ?, 1, ?, ?)`,
			name, name+"-secret", now.UnixMilli(), expiry.UnixMilli(), dismissed).Error; err != nil {
			t.Fatal(err)
		}
	}
	insert("soon", now.Add(48*time.Hour), 0)
	insert("later", now.Add(10*24*time.Hour), 0)
	insert("dismissed", now.Add(24*time.Hour), 1)

	h.checkNodeExpiry(now)
	msgs := sent()
	if len(msgs) != 1 || msgs[0].Event != notifyEventNodeExpiry || msgs[0].Title != "节点即将到期" {
		t.Fatalf("expiry alert: %v", msgs)
	}
	h.checkNodeExpiry(now.Add(time.Hour))
	if msgs := sent(); len(msgs) != 1 {
		t.Fatalf("the same expiry must alert once: %v", msgs)
	}
}
