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
			// Join with RDS route config. Deep-copy so each HCM owns its RouteConfig
			// and mutations in future phases cannot corrupt other filter chains sharing
			// the same route_config_name.
			if rc, ok := routeConfigs[hcm.RouteConfigName]; ok {
				hcm.RouteConfig = cloneRouteConfig(rc)
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
					Prefix:              rr.Match.Prefix,
					PathSeparatedPrefix: rr.Match.PathSeparatedPrefix,
					Path:                rr.Match.Path,
					Regex:               rr.Match.SafeRegex.Regex,
				},
			}

			// Extract cluster from the route action
			if rr.Route.Cluster != "" {
				route.Cluster = rr.Route.Cluster
			}

			// Extract URLRewrite from regex_rewrite on the route action
			if rr.Route.RegexRewrite.Pattern.Regex != "" {
				route.Rewrite = &model.RouteRewrite{
					RegexPattern: rr.Route.RegexRewrite.Pattern.Regex,
					Substitution: rr.Route.RegexRewrite.Substitution,
				}
			}

			// Extract mirror clusters from request_mirror_policies
			for _, mp := range rr.Route.RequestMirrorPolicies {
				if mp.Cluster != "" {
					route.MirrorClusters = append(route.MirrorClusters, mp.Cluster)
				}
			}

			// Extract request headers to add (HTTPRouteFilter RequestHeaderModifier)
			for _, h := range rr.RequestHeadersToAdd {
				route.RequestHeadersToAdd = append(route.RequestHeadersToAdd, model.HeaderOperation{
					Key:   h.Header.Key,
					Value: h.Header.Value,
				})
			}

			// Extract response headers to add (HTTPRouteFilter ResponseHeaderModifier)
			for _, h := range rr.ResponseHeadersToAdd {
				route.ResponseHeadersToAdd = append(route.ResponseHeadersToAdd, model.HeaderOperation{
					Key:   h.Header.Key,
					Value: h.Header.Value,
				})
			}

			// Extract response headers to remove
			route.ResponseHeadersToRemove = rr.ResponseHeadersToRemove

			// Extract header matches (Exact and RegularExpression)
			for _, hm := range rr.Match.Headers {
				if hm.StringMatch.SafeRegex.Regex != "" {
					route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
						Name:  hm.Name,
						Value: hm.StringMatch.SafeRegex.Regex,
						Regex: true,
					})
				} else {
					route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
						Name:  hm.Name,
						Value: hm.StringMatch.Exact,
					})
				}
			}

			// Extract query parameter matches (Exact and RegularExpression)
			for _, qp := range rr.Match.QueryParameters {
				if qp.StringMatch.SafeRegex.Regex != "" {
					route.Match.QueryParams = append(route.Match.QueryParams, model.QueryParamMatch{
						Name:  qp.Name,
						Value: qp.StringMatch.SafeRegex.Regex,
						Regex: true,
					})
				} else {
					route.Match.QueryParams = append(route.Match.QueryParams, model.QueryParamMatch{
						Name:  qp.Name,
						Value: qp.StringMatch.Exact,
					})
				}
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
		Prefix              string `json:"prefix"`
		PathSeparatedPrefix string `json:"path_separated_prefix"`
		Path                string `json:"path"`
		// safe_regex is used for RegularExpression path matchers from the K8S Gateway API
		SafeRegex struct {
			Regex string `json:"regex"`
		} `json:"safe_regex"`
		Headers []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact    string `json:"exact"`
				SafeRegex struct {
					Regex string `json:"regex"`
				} `json:"safe_regex"`
			} `json:"string_match"`
		} `json:"headers"`
		// query_parameters captures K8S Gateway API QueryParam matchers
		QueryParameters []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact    string `json:"exact"`
				SafeRegex struct {
					Regex string `json:"regex"`
				} `json:"safe_regex"`
			} `json:"string_match"`
		} `json:"query_parameters"`
	} `json:"match"`
	Route struct {
		Cluster string `json:"cluster"`
		// TODO: handle weighted_clusters for traffic-split routes (Phase 1 scope: direct cluster only)
		RequestMirrorPolicies []struct {
			Cluster string `json:"cluster"`
		} `json:"request_mirror_policies"`
		// regex_rewrite captures URLRewrite HTTPRouteFilter path rewrites
		RegexRewrite struct {
			Pattern struct {
				Regex string `json:"regex"`
			} `json:"pattern"`
			Substitution string `json:"substitution"`
		} `json:"regex_rewrite"`
	} `json:"route"`
	// Native Envoy header manipulation (used by HTTPRouteFilter RequestHeaderModifier / ResponseHeaderModifier)
	RequestHeadersToAdd []struct {
		Header struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"header"`
	} `json:"request_headers_to_add"`
	ResponseHeadersToAdd []struct {
		Header struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"header"`
	} `json:"response_headers_to_add"`
	ResponseHeadersToRemove []string                   `json:"response_headers_to_remove"`
	TypedPerFilterConfig    map[string]json.RawMessage `json:"typed_per_filter_config"`
	Metadata                *rawRouteMetadata          `json:"metadata"`
}

type rawRouteMetadata struct {
	FilterMetadata map[string]json.RawMessage `json:"filter_metadata"`
}

// cloneRouteConfig returns a deep copy of rc so each HCM owns its own RouteConfig.
// This prevents mutations in later phases from corrupting filter chains that share
// the same route_config_name.
func cloneRouteConfig(rc *model.RouteConfig) *model.RouteConfig {
	clone := &model.RouteConfig{
		Name:         rc.Name,
		VirtualHosts: make([]model.VirtualHost, len(rc.VirtualHosts)),
	}
	for i, vh := range rc.VirtualHosts {
		cloneVH := model.VirtualHost{
			Name:    vh.Name,
			Domains: append([]string(nil), vh.Domains...),
			Routes:  make([]model.Route, len(vh.Routes)),
		}
		for j, r := range vh.Routes {
			cloneRoute := r // copy all scalar fields
			// Deep-copy slice fields
			cloneRoute.MirrorClusters = append([]string(nil), r.MirrorClusters...)
			cloneRoute.RequestHeadersToAdd = append([]model.HeaderOperation(nil), r.RequestHeadersToAdd...)
			cloneRoute.ResponseHeadersToAdd = append([]model.HeaderOperation(nil), r.ResponseHeadersToAdd...)
			cloneRoute.ResponseHeadersToRemove = append([]string(nil), r.ResponseHeadersToRemove...)
			cloneRoute.Match.Headers = append([]model.HeaderMatch(nil), r.Match.Headers...)
			cloneRoute.Match.QueryParams = append([]model.QueryParamMatch(nil), r.Match.QueryParams...)
			// Rewrite is a pointer — copy the struct value if set
			if r.Rewrite != nil {
				rw := *r.Rewrite
				cloneRoute.Rewrite = &rw
			}
			cloneVH.Routes[j] = cloneRoute
		}
		clone.VirtualHosts[i] = cloneVH
	}
	return clone
}
