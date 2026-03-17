package model_test

import (
	"encoding/json"
	"testing"

	"github.com/DuncanDoyle/krp/internal/model"
)

func TestEnvoySnapshotJSONRoundtrip(t *testing.T) {
	snapshot := model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "0.0.0.0:80",
				FilterChains: []model.NetworkFilterChain{
					{
						Name: "listener~80",
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "listener~80~api_example_com",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Name:    "listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0",
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got model.EnvoySnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(got.Listeners))
	}
	l := got.Listeners[0]
	if l.Name != "listener~80" {
		t.Errorf("expected listener name 'listener~80', got %q", l.Name)
	}
	if l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0].Cluster != "kube_httpbin_httpbin_8000" {
		t.Error("cluster name mismatch after roundtrip")
	}
}

func TestEnvoySnapshotTLS(t *testing.T) {
	snapshot := model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~443",
				Address: "0.0.0.0:443",
				FilterChains: []model.NetworkFilterChain{
					{
						Name: "https-api",
						TLS: &model.TLSContext{
							SNIHosts: []string{"api.example.com"},
						},
						HCM: &model.HCMConfig{
							RouteConfigName: "https-api",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got model.EnvoySnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	fc := got.Listeners[0].FilterChains[0]
	if fc.TLS == nil {
		t.Fatal("expected TLS context")
	}
	if fc.TLS.SNIHosts[0] != "api.example.com" {
		t.Errorf("expected SNI host 'api.example.com', got %q", fc.TLS.SNIHosts[0])
	}
	if !fc.HCM.HTTPFilters[0].Disabled {
		t.Error("expected first filter to be disabled")
	}
}
