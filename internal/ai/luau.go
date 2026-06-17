package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gg582/nozik/internal/model"
	lua "github.com/yuin/gopher-lua"
)

// LuauCustomConfig holds a user-defined "Others" provider.
type LuauCustomConfig struct {
	Name       string            `json:"name"`
	ScriptPath string            `json:"script_path"`
	APIKey     string            `json:"api_key,omitempty"`
	Defaults   map[string]string `json:"defaults,omitempty"`
}

// LuauCustomStrategy runs a user-provided Luau script to call an arbitrary endpoint.
type LuauCustomStrategy struct {
	Config LuauCustomConfig
	Client *http.Client
}

// NewLuauCustomStrategy creates a strategy from the supplied config.
func NewLuauCustomStrategy(cfg LuauCustomConfig) *LuauCustomStrategy {
	return &LuauCustomStrategy{Config: cfg, Client: http.DefaultClient}
}

func (l *LuauCustomStrategy) Name() string {
	if l.Config.Name != "" {
		return l.Config.Name
	}
	return "Others"
}

func (l *LuauCustomStrategy) RecommendTracks(ctx context.Context, req RecommendationRequest) ([]model.Recommendation, error) {
	if l.Config.ScriptPath == "" {
		return nil, fmt.Errorf("no Luau script configured for custom provider")
	}
	script, err := os.ReadFile(l.Config.ScriptPath)
	if err != nil {
		return nil, fmt.Errorf("reading luau script: %w", err)
	}

	L := lua.NewState()
	defer L.Close()

	// Expose a synchronous HTTP helper to Luau.
	L.SetGlobal("http_request", L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		method := strings.ToUpper(L.CheckString(2))
		headersTbl := L.CheckTable(3)
		body := L.CheckString(4)
		headers := make(map[string]string)
		headersTbl.ForEach(func(k, v lua.LValue) {
			headers[k.String()] = v.String()
		})
		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}
		hreq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			L.Push(lua.LString(""))
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 3
		}
		for k, v := range headers {
			hreq.Header.Set(k, v)
		}
		resp, err := l.Client.Do(hreq)
		if err != nil {
			L.Push(lua.LString(""))
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 3
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		L.Push(lua.LString(string(b)))
		L.Push(lua.LNumber(resp.StatusCode))
		L.Push(lua.LString(""))
		return 3
	}))

	// Provide config defaults and API key as a table.
	cfgTbl := L.NewTable()
	cfgTbl.RawSetString("api_key", lua.LString(l.Config.APIKey))
	defaults := L.NewTable()
	for k, v := range l.Config.Defaults {
		defaults.RawSetString(k, lua.LString(v))
	}
	cfgTbl.RawSetString("defaults", defaults)
	L.SetGlobal("config", cfgTbl)

	if err := L.DoString(string(script)); err != nil {
		return nil, fmt.Errorf("running luau script: %w", err)
	}

	// Call build_request(message, emotion).
	if err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("build_request"),
		NRet:    1,
		Protect: true,
	}, lua.LString(req.Message), lua.LString(string(req.Emotion))); err != nil {
		return nil, fmt.Errorf("build_request: %w", err)
	}
	reqTbl := L.Get(-1)
	L.Pop(1)
	reqTable, ok := reqTbl.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("build_request must return a table")
	}
	url := tableString(reqTable, "url")
	method := tableString(reqTable, "method")
	body := tableString(reqTable, "body")
	headersTbl := reqTable.RawGetString("headers")
	var headers map[string]string
	if ht, ok := headersTbl.(*lua.LTable); ok {
		headers = make(map[string]string)
		ht.ForEach(func(k, v lua.LValue) {
			headers[k.String()] = v.String()
		})
	}

	hreq, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		hreq.Header.Set(k, v)
	}
	resp, err := l.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("custom provider http %d: %s", resp.StatusCode, string(respBody))
	}

	// Call parse_response(body).
	if err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("parse_response"),
		NRet:    1,
		Protect: true,
	}, lua.LString(string(respBody))); err != nil {
		return nil, fmt.Errorf("parse_response: %w", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	tbl, ok := result.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("parse_response must return a table (array)")
	}
	return luaToRecommendations(tbl)
}

func tableString(t *lua.LTable, key string) string {
	v := t.RawGetString(key)
	if v == nil || v.Type() == lua.LTNil {
		return ""
	}
	return v.String()
}

func luaToRecommendations(tbl *lua.LTable) ([]model.Recommendation, error) {
	var recs []model.Recommendation
	tbl.ForEach(func(_, v lua.LValue) {
		sub, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		r := model.Recommendation{
			Title:           tableString(sub, "title"),
			Artist:          tableString(sub, "artist"),
			Album:           tableString(sub, "album"),
			Reason:          tableString(sub, "reason"),
			ProviderID:      tableString(sub, "provider_id"),
			ProviderTrackID: tableString(sub, "provider_track_id"),
		}
		if r.ProviderID == "" {
			r.ProviderID = "yt-music"
		}
		recs = append(recs, r)
	})
	if len(recs) == 0 {
		return nil, fmt.Errorf("no recommendations parsed from custom response")
	}
	return recs, nil
}
