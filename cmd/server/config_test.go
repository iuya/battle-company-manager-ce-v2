package main

import (
	"strings"
	"testing"
	"time"

	"go-simpler.org/env"
)

func TestConfigFromEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr string
	}{
		{"default", nil, 16, ""},
		{"set", map[string]string{"MAX_CACHED_RULESETS": "4"}, 4, ""},
		{"zero is refused", map[string]string{"MAX_CACHED_RULESETS": "0"}, 0, "MAX_CACHED_RULESETS"},
		{"negative is refused", map[string]string{"MAX_CACHED_RULESETS": "-2"}, 0, "MAX_CACHED_RULESETS"},
		{"not a number", map[string]string{"MAX_CACHED_RULESETS": "many"}, 0, "parsing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg config
			err := env.Load(&cfg, &env.Options{Source: env.Map(envMap(tt.env))})
			if err == nil {
				err = cfg.validate()
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MaxCachedRuleSets != tt.want {
				t.Errorf("MaxCachedRuleSets = %d, want %d", cfg.MaxCachedRuleSets, tt.want)
			}
		})
	}
}

func envMap(m map[string]string) map[string]string {
	out := map[string]string{"DATABASE_URL": "postgres://x"}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestRulesBuildTimeoutFromEnvironment(t *testing.T) {
	for _, tt := range []struct {
		val     string
		want    time.Duration
		wantErr bool
	}{{"", time.Minute, false}, {"90s", 90 * time.Second, false}, {"500ms", 0, true}, {"0s", 0, true}, {"-5s", 0, true}} {
		var cfg config
		m := map[string]string{}
		if tt.val != "" {
			m["RULES_BUILD_TIMEOUT"] = tt.val
		}
		err := env.Load(&cfg, &env.Options{Source: env.Map(envMap(m))})
		if err == nil {
			err = cfg.validate()
		}
		if (err != nil) != tt.wantErr || (err == nil && cfg.RulesBuildTimeout != tt.want) {
			t.Errorf("%q: timeout %s, err %v", tt.val, cfg.RulesBuildTimeout, err)
		}
	}
}
