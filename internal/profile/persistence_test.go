package profile

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkspacePersistencePoliciesRoundTrip(t *testing.T) {
	for _, strategy := range []WorkspaceStrategy{WorkspaceStrategySequential, WorkspaceStrategyInterleave} {
		for _, all := range []bool{false, true} {
			p := New("test", []OutputConfig{{Key: "a", Name: "DP-1", Enabled: true, Scale: 1}, {Key: "b", Name: "DP-2", Enabled: true, Scale: 1}})
			p.Workspaces = WorkspaceSettings{Enabled: true, Strategy: strategy, MaxWorkspaces: 8, GroupSize: 2, MonitorOrder: []string{"a", "b"}, PersistAll: all}
			rules := ResolveWorkspaceRules(p, nil)
			defaults := 0
			for _, rule := range rules {
				if rule.Default {
					defaults++
				}
				if rule.Persistent != (all || rule.Default) {
					t.Fatalf("%s all=%v: %+v", strategy, all, rule)
				}
			}
			if defaults != 2 {
				t.Fatalf("expected one default per display: %+v", rules)
			}
			inferred, ok := inferGeneratedWorkspaceSettings(rules)
			if !ok || inferred.PersistAll != all {
				t.Fatalf("readback lost persistence: %+v", inferred)
			}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var restored Profile
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ResolveWorkspaceRules(restored, nil), rules) {
				t.Fatal("JSON roundtrip changed plan")
			}
			p.Workspaces.Strategy = WorkspaceStrategyManual
			p.Workspaces.Rules = rules
			p.Workspaces.PersistAll = !all
			if !reflect.DeepEqual(ResolveWorkspaceRules(p, nil), rules) {
				t.Fatal("global policy overrode manual rules")
			}
		}
	}
}
