// Tests for the unexported resolveFilterConfig helper. This file uses
// package renderer (not renderer_test) to access the unexported function directly.
// TestMain is declared in renderer_test.go and applies to all test files in
// this directory, so the ANSI color profile is already forced there.
package renderer

import (
	"testing"

	"github.com/DuncanDoyle/krp/internal/model"
)

// TestResolveFilterConfig covers the config-resolution priority rules for
// [resolveFilterConfig]:
//  1. Per-route config (TypedPerFilterConfig[filter.Name]) takes precedence
//     over HCM-level config (HTTPFilter.TypedConfig) when the value is non-nil.
//  2. Per-route config key present but value nil → falls through to HCM config.
//  3. Neither source set → returns nil.
//  4. HCM-level config set, no per-route config → returns HCM config.
//  5. Empty map per-route value → treated as "has config" (non-nil in Go) and returned as-is.
func TestResolveFilterConfig(t *testing.T) {
	hmcConfig := map[string]any{"@type": "type.googleapis.com/hcm.Config", "level": "hcm"}
	perRouteConfig := map[string]any{"@type": "type.googleapis.com/route.Config", "level": "route"}

	tests := []struct {
		name                 string
		filterTypedConfig    map[string]any
		typedPerFilterConfig map[string]any
		filterName           string
		wantNil              bool
		wantValue            any
	}{
		{
			name:                 "per-route config takes precedence over HCM config",
			filterName:           "io.solo.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"io.solo.transformation": perRouteConfig},
			wantValue:            perRouteConfig,
		},
		{
			name:                 "per-route key present but value nil falls through to HCM config",
			filterName:           "io.solo.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"io.solo.transformation": nil},
			wantValue:            hmcConfig,
		},
		{
			name:                 "neither per-route nor HCM config set returns nil",
			filterName:           "envoy.filters.http.router",
			filterTypedConfig:    nil,
			typedPerFilterConfig: nil,
			wantNil:              true,
		},
		{
			name:                 "HCM config set, no per-route config returns HCM config",
			filterName:           "io.solo.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: nil,
			wantValue:            hmcConfig,
		},
		{
			name:              "empty map per-route value is treated as has-config and returned",
			filterName:        "envoy.filters.http.cors",
			filterTypedConfig: hmcConfig,
			typedPerFilterConfig: map[string]any{
				"envoy.filters.http.cors": map[string]any{}, // empty map — non-nil in Go
			},
			wantValue: map[string]any{},
		},
		{
			name:                 "per-route config for different filter name does not apply",
			filterName:           "envoy.filters.http.router",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"io.solo.transformation": perRouteConfig},
			wantValue:            hmcConfig, // falls through to HCM config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := model.HTTPFilter{
				Name:        tt.filterName,
				TypedConfig: tt.filterTypedConfig,
			}
			got := resolveFilterConfig(f, tt.typedPerFilterConfig)
			if tt.wantNil {
				if got != nil {
					t.Errorf("resolveFilterConfig() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("resolveFilterConfig() = nil, want %v", tt.wantValue)
				return
			}
			// Compare by pointer identity for the map cases — the function must
			// return exactly the map it received, not a copy.
			gotMap, gotOk := got.(map[string]any)
			wantMap, wantOk := tt.wantValue.(map[string]any)
			if gotOk && wantOk {
				// Verify same backing map via length and key presence (pointer equality
				// is not reliable across interface conversions in all Go versions).
				if len(gotMap) != len(wantMap) {
					t.Errorf("resolveFilterConfig() returned map with len %d, want %d", len(gotMap), len(wantMap))
				}
				for k, wv := range wantMap {
					if gv, ok := gotMap[k]; !ok || gv != wv {
						t.Errorf("resolveFilterConfig() map[%q] = %v, want %v", k, gv, wv)
					}
				}
			}
		})
	}
}
