package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const diceToken = "test-token"

func diceReq(method, body, token, remote string) *http.Request {
	r := httptest.NewRequest(method, "/_e2e/dice", strings.NewReader(body))
	r.RemoteAddr = remote
	if token != "" {
		r.Header.Set(e2eDiceTokenHeader, token)
	}
	return r
}

func diceFixture(t *testing.T, cfg Config) (*fixture, *core.ScriptedDice) {
	t.Helper()
	f := newFixture(t, cfg)
	script := core.NewScriptedDice(&core.FixedDice{Rolls: []int{1}})
	f.srv.SetDiceScript(script)
	return f, script
}

func TestDiceScriptEndpoint(t *testing.T) {
	f, script := diceFixture(t, Config{AppEnv: "development", E2EDice: true, E2EDiceToken: diceToken})

	rec := f.do(diceReq("POST", `{"next":[3,5]}`, diceToken, "127.0.0.1:4000"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if rec = f.do(diceReq("GET", "", diceToken, "[::1]:4000")); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Errorf("GET (IPv6 loopback): %d %s", rec.Code, rec.Body)
	}
	if got := f.srv.die(6); got != 3 {
		t.Errorf("the server's die takes the queue: %d", got)
	}
	if script.Len() != 1 {
		t.Errorf("one result left: %d", script.Len())
	}
	if rec = f.do(diceReq("DELETE", "", diceToken, "127.0.0.1:4000")); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":0`) || script.Len() != 0 {
		t.Errorf("DELETE: %d %s", rec.Code, rec.Body)
	}
	if got := f.srv.die(6); got != 1 {
		t.Errorf("an empty queue falls back: %d", got)
	}
}

func TestDiceScriptIsGuarded(t *testing.T) {
	f, script := diceFixture(t, Config{AppEnv: "development", E2EDice: true, E2EDiceToken: diceToken})
	for _, tc := range []struct {
		name   string
		r      *http.Request
		status int
	}{
		{"no token", diceReq("POST", `{"next":[3]}`, "", "127.0.0.1:1"), 403},
		{"wrong token", diceReq("POST", `{"next":[3]}`, "nope", "127.0.0.1:1"), 403},
		{"not loopback", diceReq("POST", `{"next":[3]}`, diceToken, "192.0.2.1:1"), 403},
		{"not loopback delete", diceReq("DELETE", "", diceToken, "10.0.0.5:1"), 403},
		{"bad body", diceReq("POST", `{"next":"x"}`, diceToken, "127.0.0.1:1"), 400},
		{"empty queue", diceReq("POST", `{"next":[]}`, diceToken, "127.0.0.1:1"), 400},
		{"unknown field", diceReq("POST", `{"next":[1],"x":1}`, diceToken, "127.0.0.1:1"), 400},
	} {
		if rec := f.do(tc.r); rec.Code != tc.status {
			t.Errorf("%s: %d, want %d", tc.name, rec.Code, tc.status)
		}
	}
	if script.Len() != 0 {
		t.Errorf("nothing was queued: %d", script.Len())
	}
}

func TestDiceScriptDoesNotExistWithoutTheSwitch(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"production with the switch", Config{AppEnv: "production", E2EDice: true, E2EDiceToken: diceToken}},
		{"production", Config{AppEnv: "production"}},
		{"development without the switch", Config{AppEnv: "development", E2EDiceToken: diceToken}},
		{"development without a token", Config{AppEnv: "development", E2EDice: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, script := diceFixture(t, tc.cfg)
			for _, m := range []string{"GET", "POST", "DELETE"} {
				if rec := f.do(diceReq(m, `{"next":[3]}`, diceToken, "127.0.0.1:1")); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s answers %d, want the route absent", m, rec.Code)
				}
			}
			script.Push(6)
			if f.srv.die == nil || f.srv.diceScript != nil {
				t.Error("the server keeps its own die")
			}
			if script.Len() != 1 {
				t.Error("the script was not touched")
			}
		})
	}
}

func TestConfigRefusesTheDiceScriptInProductionOrWithoutToken(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"production":           {Config{AppEnv: "production", CSPMode: "report", E2EDice: true, E2EDiceToken: "t"}, true},
		"development no token": {Config{AppEnv: "development", CSPMode: "report", E2EDice: true}, true},
		"development":          {Config{AppEnv: "development", CSPMode: "report", E2EDice: true, E2EDiceToken: "t"}, false},
		"off in production":    {Config{AppEnv: "production", CSPMode: "report"}, false},
	} {
		if err := tc.cfg.Resolve(); (err != nil) != tc.wantErr {
			t.Errorf("%s: Resolve() = %v, wantErr %v", name, err, tc.wantErr)
		}
	}
}
