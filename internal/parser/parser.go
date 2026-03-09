package parser

import (
	"encoding/json"
	"fmt"

	"github.com/DuncanDoyle/kfp/internal/model"
)

// Parse takes raw Envoy /config_dump JSON bytes and returns an EnvoySnapshot.
// It joins listeners with their RDS route configs by matching route_config_name.
func Parse(data []byte) (*model.EnvoySnapshot, error) {
	var dump configDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("parsing config dump JSON: %w", err)
	}

	// Parse each config section by @type
	var listeners []rawListener
	routeConfigs := map[string]*model.RouteConfig{} // keyed by name

	for _, raw := range dump.Configs {
		var typed typedConfig
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}

		switch typed.Type {
		case "type.googleapis.com/envoy.admin.v3.ListenersConfigDump":
			var ld listenersConfigDump
			if err := json.Unmarshal(raw, &ld); err != nil {
				continue
			}
			listeners = ld.DynamicListeners

		case "type.googleapis.com/envoy.admin.v3.RoutesConfigDump":
			var rd routesConfigDump
			if err := json.Unmarshal(raw, &rd); err != nil {
				continue
			}
			for _, drc := range rd.DynamicRouteConfigs {
				rc := parseRouteConfig(drc.RouteConfig)
				routeConfigs[rc.Name] = rc
			}
		}
	}

	// Build the EnvoySnapshot by joining listeners with route configs
	snapshot := &model.EnvoySnapshot{}
	for _, rl := range listeners {
		l := parseListener(rl, routeConfigs)
		snapshot.Listeners = append(snapshot.Listeners, l)
	}

	return snapshot, nil
}

// parseListener converts a raw dynamic listener into the model.Listener,
// joining each HCM to its route config via route_config_name.
func parseListener(rl rawListener, routeConfigs map[string]*model.RouteConfig) model.Listener {
	l := model.Listener{
		Name: rl.Name,
	}

	if rl.ActiveState.Listener.Address.SocketAddress.Address != "" {
		l.Address = fmt.Sprintf("%s:%d",
			rl.ActiveState.Listener.Address.SocketAddress.Address,
			rl.ActiveState.Listener.Address.SocketAddress.PortValue,
		)
	}

	for _, rfc := range rl.ActiveState.Listener.FilterChains {
		nfc := model.NetworkFilterChain{
			Name: rfc.Name,
		}

		// TLS context from filter_chain_match.server_names
		if len(rfc.FilterChainMatch.ServerNames) > 0 {
			nfc.TLS = &model.TLSContext{
				SNIHosts: rfc.FilterChainMatch.ServerNames,
			}
		}

		// Find the HCM in the network filters
		for _, nf := range rfc.Filters {
			if nf.Name != "envoy.filters.network.http_connection_manager" {
				continue
			}
			hcm := parseHCM(nf.TypedConfig)
			if hcm == nil {
				continue
			}
			// Join with RDS route config
			if rc, ok := routeConfigs[hcm.RouteConfigName]; ok {
				hcm.RouteConfig = rc
			}
			nfc.HCM = hcm
		}

		l.FilterChains = append(l.FilterChains, nfc)
	}

	return l
}

// parseHCM extracts the HCM config from the raw typed_config JSON.
func parseHCM(raw json.RawMessage) *model.HCMConfig {
	var hcm rawHCM
	if err := json.Unmarshal(raw, &hcm); err != nil {
		return nil
	}

	result := &model.HCMConfig{
		RouteConfigName: hcm.RDS.RouteConfigName,
	}

	for _, hf := range hcm.HTTPFilters {
		filter := model.HTTPFilter{
			Name:     hf.Name,
			Disabled: hf.Disabled,
		}
		// Store the raw typed config for later phases
		if len(hf.TypedConfig) > 0 {
			var tc map[string]any
			if err := json.Unmarshal(hf.TypedConfig, &tc); err == nil {
				filter.TypedConfig = tc
			}
		}
		result.HTTPFilters = append(result.HTTPFilters, filter)
	}

	return result
}

// parseRouteConfig converts a raw route config into the model.
func parseRouteConfig(raw rawRouteConfig) *model.RouteConfig {
	rc := &model.RouteConfig{
		Name: raw.Name,
	}

	for _, rvh := range raw.VirtualHosts {
		vh := model.VirtualHost{
			Name:    rvh.Name,
			Domains: rvh.Domains,
		}

		for _, rr := range rvh.Routes {
			route := model.Route{
				Name: rr.Name,
				Match: model.RouteMatch{
					Prefix: rr.Match.Prefix,
					Path:   rr.Match.Path,
				},
			}

			// Extract cluster from the route action
			if rr.Route.Cluster != "" {
				route.Cluster = rr.Route.Cluster
			}

			// Extract header matches
			for _, hm := range rr.Match.Headers {
				value := hm.StringMatch.Exact
				route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
					Name:  hm.Name,
					Value: value,
				})
			}

			// Store per-filter config and metadata for later phases
			if len(rr.TypedPerFilterConfig) > 0 {
				tpfc := map[string]any{}
				for k, v := range rr.TypedPerFilterConfig {
					var parsed any
					if err := json.Unmarshal(v, &parsed); err == nil {
						tpfc[k] = parsed
					}
				}
				route.TypedPerFilterConfig = tpfc
			}

			if rr.Metadata != nil && len(rr.Metadata.FilterMetadata) > 0 {
				meta := map[string]any{}
				for k, v := range rr.Metadata.FilterMetadata {
					var parsed any
					if err := json.Unmarshal(v, &parsed); err == nil {
						meta[k] = parsed
					}
				}
				route.Metadata = meta
			}

			vh.Routes = append(vh.Routes, route)
		}

		rc.VirtualHosts = append(rc.VirtualHosts, vh)
	}

	return rc
}

// --- Raw JSON structs matching the actual Envoy config dump format ---

type configDump struct {
	Configs []json.RawMessage `json:"configs"`
}

type typedConfig struct {
	Type string `json:"@type"`
}

// Listeners

type listenersConfigDump struct {
	DynamicListeners []rawListener `json:"dynamic_listeners"`
}

type rawListener struct {
	Name        string `json:"name"`
	ActiveState struct {
		Listener struct {
			Name    string `json:"name"`
			Address struct {
				SocketAddress struct {
					Address   string `json:"address"`
					PortValue int    `json:"port_value"`
				} `json:"socket_address"`
			} `json:"address"`
			FilterChains []rawFilterChain `json:"filter_chains"`
		} `json:"listener"`
	} `json:"active_state"`
}

type rawFilterChain struct {
	Name             string `json:"name"`
	FilterChainMatch struct {
		ServerNames []string `json:"server_names"`
	} `json:"filter_chain_match"`
	Filters []rawNetworkFilter `json:"filters"`
}

type rawNetworkFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

// HCM

type rawHCM struct {
	RDS struct {
		RouteConfigName string `json:"route_config_name"`
	} `json:"rds"`
	HTTPFilters []rawHTTPFilter `json:"http_filters"`
}

type rawHTTPFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
	Disabled    bool            `json:"disabled"`
}

// Routes

type routesConfigDump struct {
	DynamicRouteConfigs []struct {
		RouteConfig rawRouteConfig `json:"route_config"`
	} `json:"dynamic_route_configs"`
}

type rawRouteConfig struct {
	Name         string           `json:"name"`
	VirtualHosts []rawVirtualHost `json:"virtual_hosts"`
}

type rawVirtualHost struct {
	Name    string     `json:"name"`
	Domains []string   `json:"domains"`
	Routes  []rawRoute `json:"routes"`
}

type rawRoute struct {
	Name  string `json:"name"`
	Match struct {
		Prefix  string `json:"prefix"`
		Path    string `json:"path"`
		Headers []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact string `json:"exact"`
			} `json:"string_match"`
		} `json:"headers"`
	} `json:"match"`
	Route struct {
		Cluster string `json:"cluster"`
		// TODO: handle weighted_clusters for traffic-split routes (Phase 1 scope: direct cluster only)
	} `json:"route"`
	TypedPerFilterConfig map[string]json.RawMessage `json:"typed_per_filter_config"`
	Metadata             *rawRouteMetadata          `json:"metadata"`
}

type rawRouteMetadata struct {
	FilterMetadata map[string]json.RawMessage `json:"filter_metadata"`
}
