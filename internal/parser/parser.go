// Package parser converts a raw Envoy /config_dump JSON payload into the
// domain model defined in internal/model. The Envoy config_dump is a heterogeneous
// JSON array where each element has a "@type" discriminator; the parser dispatches
// on that field to extract ListenersConfigDump and RoutesConfigDump sections, then
// joins them into an [model.EnvoySnapshot].
//
// Non-fatal errors (e.g. a single malformed section) are collected as warnings and
// returned alongside the best-effort snapshot so the caller can surface them to the
// user rather than silently dropping data.
package parser

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/DuncanDoyle/kfp/internal/model"
)

// ParseResult holds the parsed [model.EnvoySnapshot] and any non-fatal warnings
// collected during parsing (e.g. malformed config sections that were skipped).
// Warnings are surfaced to the user so that silently dropped sections are visible.
type ParseResult struct {
	Snapshot *model.EnvoySnapshot
	Warnings []string
}

// Parse takes raw Envoy /config_dump JSON bytes and returns a ParseResult.
// Non-fatal errors (e.g. a single malformed config section) are collected as
// warnings rather than aborting the parse, so the caller receives a best-effort
// snapshot alongside a description of what was skipped.
func Parse(data []byte) (ParseResult, error) {
	var dump configDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return ParseResult{}, fmt.Errorf("parsing config dump JSON: %w", err)
	}

	// Parse each config section by @type, collecting warnings for any that fail.
	var listeners []rawListener
	routeConfigs := map[string]*model.RouteConfig{} // keyed by name
	var warnings []string

	for _, raw := range dump.Configs {
		var typed typedConfig
		if err := json.Unmarshal(raw, &typed); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped config section: cannot read @type: %v", err))
			continue
		}

		switch typed.Type {
		case "type.googleapis.com/envoy.admin.v3.ListenersConfigDump":
			var ld listenersConfigDump
			if err := json.Unmarshal(raw, &ld); err != nil {
				warnings = append(warnings, fmt.Sprintf("skipped ListenersConfigDump: %v", err))
				continue
			}
			listeners = ld.DynamicListeners

		case "type.googleapis.com/envoy.admin.v3.RoutesConfigDump":
			var rd routesConfigDump
			if err := json.Unmarshal(raw, &rd); err != nil {
				warnings = append(warnings, fmt.Sprintf("skipped RoutesConfigDump: %v", err))
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

	return ParseResult{Snapshot: snapshot, Warnings: warnings}, nil
}

// parseListener converts a raw dynamic listener into the model.Listener,
// joining each HCM to its route config via `route_config_name`.
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

// parseHCM extracts the HCM config from the raw `typed_config` JSON.
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
//
// These types are intentionally minimal: only the fields that kfp currently
// reads are declared. Unknown fields are silently ignored by encoding/json,
// which keeps these structs stable against Envoy API additions.

// configDump is the top-level /config_dump response. Each element of Configs
// is a distinct config section identified by its "@type" field.
type configDump struct {
	Configs []json.RawMessage `json:"configs"`
}

// typedConfig is used to peek at the "@type" discriminator before fully
// unmarshalling a config section into its concrete type.
type typedConfig struct {
	Type string `json:"@type"`
}

// Listeners

// listenersConfigDump corresponds to type.googleapis.com/envoy.admin.v3.ListenersConfigDump.
// Only `dynamic_listeners` are read; static listeners (bootstrap) are ignored because
// kgateway does not use them.
type listenersConfigDump struct {
	DynamicListeners []rawListener `json:"dynamic_listeners"`
}

// rawListener is a single entry from `dynamic_listeners`.
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

// rawFilterChain is a single filter_chain within a listener.
type rawFilterChain struct {
	Name             string `json:"name"`
	FilterChainMatch struct {
		ServerNames []string `json:"server_names"`
	} `json:"filter_chain_match"`
	Filters []rawNetworkFilter `json:"filters"`
}

// rawNetworkFilter is a single entry in a filter chain's `filters` list.
type rawNetworkFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

// HCM

// rawHCM represents the `typed_config` of an `envoy.filters.network.http_connection_manager`
// network filter. Only the `rds` and `http_filters` fields are extracted; all other HCM
// options (timeouts, access log, tracing, etc.) are ignored.
type rawHCM struct {
	RDS struct {
		RouteConfigName string `json:"route_config_name"`
	} `json:"rds"`
	HTTPFilters []rawHTTPFilter `json:"http_filters"`
}

// rawHTTPFilter is a single entry in the HCM `http_filters` list.
type rawHTTPFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
	Disabled    bool            `json:"disabled"`
}

// Routes

// routesConfigDump corresponds to type.googleapis.com/envoy.admin.v3.RoutesConfigDump.
// Only `dynamic_route_configs` are read; static route configs are ignored.
type routesConfigDump struct {
	DynamicRouteConfigs []struct {
		RouteConfig rawRouteConfig `json:"route_config"`
	} `json:"dynamic_route_configs"`
}

// rawRouteConfig is the `route_config` object within a dynamic route config entry.
type rawRouteConfig struct {
	Name         string           `json:"name"`
	VirtualHosts []rawVirtualHost `json:"virtual_hosts"`
}

// rawVirtualHost is a single virtual_host within a route config.
type rawVirtualHost struct {
	Name    string     `json:"name"`
	Domains []string   `json:"domains"`
	Routes  []rawRoute `json:"routes"`
}

// rawRoute is a single route entry within a virtual host.
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

// rawRouteMetadata holds the `metadata` object on a route, used for EKTP policy cross-references.
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
			// Deep-copy map fields so mutations in later phases (e.g. Phase 2 filter expansion)
			// cannot corrupt other HCMs that share the same route_config_name.
			// Values are opaque JSON (any) and are only read by the renderer, so shallow value copy is safe.
			if r.TypedPerFilterConfig != nil {
				tpfc := make(map[string]any, len(r.TypedPerFilterConfig))
				maps.Copy(tpfc, r.TypedPerFilterConfig)
				cloneRoute.TypedPerFilterConfig = tpfc
			}
			if r.Metadata != nil {
				meta := make(map[string]any, len(r.Metadata))
				maps.Copy(meta, r.Metadata)
				cloneRoute.Metadata = meta
			}
			cloneVH.Routes[j] = cloneRoute
		}
		clone.VirtualHosts[i] = cloneVH
	}
	return clone
}
