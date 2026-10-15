package contract_test

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"go-backend/internal/auth"
)

func TestExternalInboundImportContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	admin, _ := auth.GenerateToken(1, "admin_user", 0, secret)
	relayNode := seedKernelNode(t, r, "relay")

	good := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@198.51.100.20:8388#%E8%90%BD%E5%9C%B0"
	links := good + `\nvmess://abc\n\n`
	out := kernelContractPost(t, router, admin, "/api/v1/inbound/import", `{"links":"`+links+`"}`)
	if out.Code != 0 {
		t.Fatalf("import: %+v", out)
	}
	data := out.Data.(map[string]interface{})
	if len(data["created"].([]interface{})) != 1 || len(data["errors"].([]interface{})) != 1 ||
		!strings.Contains(data["errors"].([]interface{})[0].(string), "第 2 行") {
		t.Fatalf("one link imported, one rejected with its line: %v", data)
	}
	id := int64(data["created"].([]interface{})[0].(float64))

	dup := kernelContractPost(t, router, admin, "/api/v1/inbound/import", `{"links":"`+good+`"}`)
	if dup.Code == 0 || !strings.Contains(dup.Msg, "已经导入过") {
		t.Fatalf("re-importing the same address must be skipped: %+v", dup)
	}

	list := kernelContractPost(t, router, admin, "/api/v1/inbound/list", `{}`)
	item := list.Data.([]interface{})[0].(map[string]interface{})
	if item["external"] != true || item["name"] != "落地" || item["listen"] != "198.51.100.20" || valueAsInt(item["port"]) != 8388 {
		t.Fatalf("list item: %v", item)
	}

	share := kernelContractPost(t, router, admin, "/api/v1/inbound/share", `{"id":`+jsonInt64(id)+`}`)
	entry := share.Data.([]interface{})[0].(map[string]interface{})
	if !strings.HasPrefix(entry["uri"].(string), "ss://") || entry["host"] != "198.51.100.20" {
		t.Fatalf("direct share: %v", entry)
	}

	// A port-forward relay points the forward at the imported address.
	_, forwardID := seedRelayForward(t, r, 1, relayNode, 0, 30005, id)
	sub := kernelContractPost(t, router, admin, "/api/v1/inbound/sub/info", `{}`)
	token := sub.Data.(map[string]interface{})["token"].(string)
	res := getSub(t, router, "/api/v1/open_api/sub/"+token+"?format=clash", "")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "198.51.100.20") || !strings.Contains(res.Body.String(), "30005") {
		t.Fatalf("subscription must carry the direct and relay lines:\n%s", res.Body.String())
	}

	update := func(body string) string {
		return kernelContractPost(t, router, admin, "/api/v1/inbound/external/update", body).Msg
	}
	if msg := update(`{"id":` + jsonInt64(id) + `,"name":"x","host":"127.0.0.1","port":1,"method":"aes-256-gcm","password":"p"}`); !strings.Contains(msg, "本机") {
		t.Fatalf("loopback address must be rejected: %s", msg)
	}
	if msg := update(`{"id":` + jsonInt64(id) + `,"name":"x","host":"1.2.3.4","port":1,"method":"rc4","password":"p"}`); !strings.Contains(msg, "加密方式") {
		t.Fatalf("bad method must be rejected: %s", msg)
	}
	update(`{"id":` + jsonInt64(id) + `,"name":"新落地","host":"198.51.100.21","port":9000,"method":"chacha20-ietf-poly1305","password":"p2","relayOnly":true}`)
	if got := mustQueryString(t, r, `SELECT remote_addr FROM forward WHERE id = ?`, forwardID); got != "198.51.100.21:9000" {
		t.Fatalf("relay forward must follow the new address, got %q", got)
	}
	res = getSub(t, router, "/api/v1/open_api/sub/"+token+"?format=clash", "")
	if strings.Contains(res.Body.String(), "server: 198.51.100.21") {
		t.Fatalf("relay-only node must not have a direct line:\n%s", res.Body.String())
	}

	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/update", `{"id":`+jsonInt64(id)+`,"name":"x","protocol":"shadowsocks","port":1}`); out.Code == 0 {
		t.Fatal("kernel update path must refuse imported nodes")
	}
	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/regenerate", `{"id":`+jsonInt64(id)+`}`); out.Code == 0 {
		t.Fatal("imported nodes have no keys to regenerate")
	}
	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/toggle", `{"id":`+jsonInt64(id)+`,"enabled":false}`); out.Code != 0 || out.Msg != "操作成功" {
		t.Fatalf("toggle: %+v", out)
	}
	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/delete", `{"id":`+jsonInt64(id)+`}`); out.Code != 0 || out.Msg != "操作成功" {
		t.Fatalf("delete: %+v", out)
	}
	if mustQueryInt(t, r, `SELECT COUNT(*) FROM node_inbound_relay`) != 0 {
		t.Fatal("deleting the node must drop its relays")
	}
}
