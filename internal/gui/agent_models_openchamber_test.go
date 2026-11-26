package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

func TestOpenChamberModelList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(home, ".config", "opencode"))
	t.Setenv("OPENCHAMBER_DATA_DIR", filepath.Join(home, ".config", "openchamber"))
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: "fallback", Name: "Fallback", Members: []string{"relay/m1", "relay/m2"}}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	agentModelsAPI(mux)
	a, err := agent.Find("openchamber")
	if err != nil {
		t.Fatal(err)
	}
	models := gateway.New().Handler()
	const ref = "group/fallback"
	for _, step := range []struct {
		name, body string
		shown      bool
	}{
		{"hide", `{"hidden":["group/fallback"]}`, false},
		{"show", `{"hidden":[]}`, true},
	} {
		t.Run(step.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent-models/openchamber", strings.NewReader(step.body)))
			if w.Code != http.StatusOK {
				t.Fatalf("save list: %d %s", w.Code, w.Body)
			}
			for _, f := range agentFields(a, a.Values()) {
				if got := slices.ContainsFunc(f.Options, func(o agent.Option) bool { return o.Ref == ref }); got != step.shown {
					t.Errorf("OpenChamber's %s picker offers %s = %v, want %v", f.Key, ref, got, step.shown)
				}
			}
			req := httptest.NewRequest("GET", "/v1/models?format=opencode2", nil)
			req.Header.Set("Authorization", "Bearer "+gateway.TokenFor("opencode"))
			w = httptest.NewRecorder()
			models.ServeHTTP(w, req)
			var got struct{ Models []gateway.OpenCodeModel }
			if err := json.Unmarshal(w.Body.Bytes(), &got); w.Code != http.StatusOK || err != nil {
				t.Fatalf("list models: %d %s (%v)", w.Code, w.Body, err)
			}
			if listed := slices.ContainsFunc(got.Models, func(m gateway.OpenCodeModel) bool { return m.ID == ref }); listed != step.shown {
				t.Errorf("OpenCode discovery lists %s = %v, want %v", ref, listed, step.shown)
			}
		})
	}
	for _, pair := range []struct{ selected, editing, field string }{
		{"openchamber", "opencode", "model"},
		{"opencode", "openchamber", "small"},
	} {
		t.Run(pair.editing+" keeps "+pair.selected+" selection", func(t *testing.T) {
			selected, err := agent.Find(pair.selected)
			if err != nil {
				t.Fatal(err)
			}
			if err := selected.Apply(pair.field, "magpie/"+ref); err != nil {
				t.Fatal(err)
			}
			if value := selected.Values()[pair.field]; value != "magpie/"+ref {
				t.Fatalf("%s %s = %q, want magpie/%s", pair.selected, pair.field, value, ref)
			}
			t.Cleanup(func() {
				if err := selected.Apply(pair.field, ""); err != nil {
					t.Error(err)
				}
			})
			next := httptest.NewRequest("POST", "/api/agent-models/"+pair.editing, strings.NewReader(`{"hidden":["group/fallback"]}`))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, next)
			var got struct{ Models []agentModelJSON }
			if err := json.Unmarshal(w.Body.Bytes(), &got); w.Code != http.StatusOK || err != nil {
				t.Fatalf("save list: %d %s (%v)", w.Code, w.Body, err)
			}
			if !slices.ContainsFunc(got.Models, func(m agentModelJSON) bool { return m.ID == ref && m.InUse && !m.Hidden }) {
				t.Errorf("%s list did not keep %s in use by %s: %+v", pair.editing, ref, pair.selected, got.Models)
			}
		})
	}
}
