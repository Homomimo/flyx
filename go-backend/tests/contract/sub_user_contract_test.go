package contract_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
)

func TestSubUserContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	admin, _ := auth.GenerateToken(1, "admin_user", 0, secret)
	landing := seedKernelNode(t, r, "landing")
	if err := r.DB().Exec(`UPDATE node SET server_ip = '198.51.100.9' WHERE id = ?`, landing).Error; err != nil {
		t.Fatal(err)
	}
	vless := seedInboundOnNode(t, router, r, admin, landing,
		`{"nodeId":`+jsonInt64(landing)+`,"kernelType":"xray","name":"东京","protocol":"vless","port":443,"settings":{"flow":"xtls-rprx-vision"},"security":{"type":"reality"}}`)
	ss := seedInboundOnNode(t, router, r, admin, landing,
		`{"nodeId":`+jsonInt64(landing)+`,"kernelType":"xray","name":"SS","protocol":"shadowsocks","port":8388}`)
	var ownerSettings struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal([]byte(mustQueryString(t, r, `SELECT settings FROM node_inbound WHERE id = ?`, vless)), &ownerSettings); err != nil || ownerSettings.UUID == "" {
		t.Fatalf("owner uuid: %v", err)
	}
	ownerUUID := ownerSettings.UUID

	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/user/save", `{"name":"小明","inboundIds":[]}`); out.Code == 0 {
		t.Fatal("a user needs at least one inbound")
	}
	out := kernelContractPost(t, router, admin, "/api/v1/inbound/user/save",
		`{"name":"小明","inboundIds":[`+jsonInt64(vless)+`,`+jsonInt64(ss)+`,999]}`)
	if out.Code != 0 {
		t.Fatalf("create user: %+v", out)
	}
	view := out.Data.(map[string]interface{})
	userID := int64(view["id"].(float64))
	path := view["path"].(string)
	if len(view["inboundIds"].([]interface{})) != 2 || len(view["sharedIds"].([]interface{})) != 1 ||
		valueAsInt(view["sharedIds"].([]interface{})[0]) != int(ss) || view["active"] != true {
		t.Fatalf("user view (unknown ids dropped, ss marked shared): %v", view)
	}

	sub := func(p string) (int, string, string) {
		resp := getSub(t, router, p+"?format=base64", "")
		body, _ := base64.StdEncoding.DecodeString(resp.Body.String())
		return resp.Code, string(body), resp.Header().Get("Subscription-Userinfo")
	}
	code, body, info := sub(path)
	userUUID := mustQueryString(t, r, `SELECT uuid FROM sub_user WHERE id = ?`, userID)
	if code != http.StatusOK || !strings.Contains(body, "vless://"+userUUID+"@") || strings.Contains(body, ownerUUID) ||
		!strings.Contains(body, "ss://") || !strings.Contains(info, "expire=0") {
		t.Fatalf("user subscription must use the user's uuid: %d %s %s", code, body, info)
	}

	// The owner's subscription keeps the owner's credentials.
	token := kernelContractPost(t, router, admin, "/api/v1/inbound/sub/info", `{}`).Data.(map[string]interface{})["token"].(string)
	if _, ownerBody, _ := sub("/api/v1/open_api/sub/" + token); strings.Contains(ownerBody, userUUID) {
		t.Fatal("owner subscription must not carry user credentials")
	}

	// Expired and disabled users are refused.
	past := time.Now().Add(-time.Hour).UnixMilli()
	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/user/save",
		`{"id":`+jsonInt64(userID)+`,"name":"小明","inboundIds":[`+jsonInt64(vless)+`],"expireTime":`+jsonInt64(past)+`}`); out.Code != 0 ||
		out.Data.(map[string]interface{})["active"] != false {
		t.Fatalf("expire user: %+v", out)
	}
	if code, _, _ := sub(path); code != http.StatusForbidden {
		t.Fatalf("expired user: %d", code)
	}
	future := time.Now().Add(48 * time.Hour).UnixMilli()
	kernelContractPost(t, router, admin, "/api/v1/inbound/user/save",
		`{"id":`+jsonInt64(userID)+`,"name":"小明","inboundIds":[`+jsonInt64(vless)+`],"expireTime":`+jsonInt64(future)+`,"enabled":false}`)
	if code, _, _ := sub(path); code != http.StatusForbidden {
		t.Fatalf("disabled user: %d", code)
	}
	kernelContractPost(t, router, admin, "/api/v1/inbound/user/save",
		`{"id":`+jsonInt64(userID)+`,"name":"小明","inboundIds":[`+jsonInt64(vless)+`],"expireTime":`+jsonInt64(future)+`,"enabled":true}`)
	if code, _, info := sub(path); code != http.StatusOK || !strings.Contains(info, "expire="+jsonInt64(future/1000)) {
		t.Fatalf("re-enabled user: %d %s", code, info)
	}

	// Reset issues a new link and new credentials.
	out = kernelContractPost(t, router, admin, "/api/v1/inbound/user/reset", `{"id":`+jsonInt64(userID)+`}`)
	newPath := out.Data.(map[string]interface{})["path"].(string)
	if code, _, _ := sub(path); code != http.StatusNotFound || newPath == path {
		t.Fatalf("old link must stop working: %d", code)
	}
	if mustQueryString(t, r, `SELECT uuid FROM sub_user WHERE id = ?`, userID) == userUUID {
		t.Fatal("reset must rotate the uuid")
	}

	list := kernelContractPost(t, router, admin, "/api/v1/inbound/user/list", `{}`)
	if len(list.Data.([]interface{})) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if out := kernelContractPost(t, router, admin, "/api/v1/inbound/user/delete", `{"id":`+jsonInt64(userID)+`}`); out.Code != 0 {
		t.Fatalf("delete: %+v", out)
	}
	if code, _, _ := sub(newPath); code != http.StatusNotFound {
		t.Fatal("deleted user's link must stop working")
	}
}
