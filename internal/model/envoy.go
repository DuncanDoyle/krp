// Package model defines the domain types that represent an Envoy configuration
// snapshot. The types are intentionally decoupled from the raw Envoy JSON format
// so that the renderer and future interactive phases are not tied to Envoy's
// wire-format field names.
//
// Data flows: parser → model → renderer.
package model

// EnvoySnapshot is the complete parsed Envoy configuration.
// Built by joining data from ListenersConfigDump and RoutesConfigDump.
type EnvoySnapshot struct {
	Listeners []Listener `json:"listeners"`
}

// Listener represents an Envoy listener (e.g. listener~80, listener~443).
type Listener struct {
	Name         string               `json:"name"`
	Address      string               `json:"address"` // e.g. "0.0.0.0:80"
	FilterChains []NetworkFilterChain `json:"filterChains"`
}

// NetworkFilterChain is a network-level filter chain within a listener.
// For HTTPS listeners, there is typically one filter chain per SNI host.
type NetworkFilterChain struct {
	Name string      `json:"name"`
	TLS  *TLSContext `json:"tls,omitempty"` // nil for plaintext
	HCM  *HCMConfig  `json:"hcm,omitempty"` // extracted from the network filter list
}

// TLSContext holds TLS/SNI information for a filter chain.
type TLSContext struct {
	SNIHosts []string `json:"sniHosts"` // from filter_chain_match.server_names
}

// HCMConfig represents the HTTP Connection Manager configuration.
type HCMConfig struct {
	RouteConfigName string       `json:"routeConfigName"`       // rds.route_config_name
	HTTPFilters     []HTTPFilter `json:"httpFilters"`           // the HTTP filter pipeline
	RouteConfig     *RouteConfig `json:"routeConfig,omitempty"` // joined from RDS section
}

// HTTPFilter is a single filter in the HTTP filter pipeline.
type HTTPFilter struct {
	Name        string         `json:"name"`                  // e.g. "io.solo.transformation"
	TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config; displayed in interactive mode (Phase 3)
	Disabled    bool           `json:"disabled,omitempty"`    // filter disabled at HCM level, enabled per-route
}

// RouteConfig is an Envoy route configuration (from RDS).
type RouteConfig struct {
	Name         string        `json:"name"`
	VirtualHosts []VirtualHost `json:"virtualHosts"`
}

// VirtualHost is an Envoy virtual host within a route config.
type VirtualHost struct {
	Name    string   `json:"name"`    // e.g. "listener~80~api_example_com"
	Domains []string `json:"domains"` // e.g. ["api.example.com"]
	Routes  []Route  `json:"routes"`
}

// Route is an Envoy route within a virtual host.
type Route struct {
	Name                    string            `json:"name"`
	Match                   RouteMatch        `json:"match"`
	Cluster                 string            `json:"cluster"`                           // backend cluster name
	Rewrite                 *RouteRewrite     `json:"rewrite,omitempty"`                 // path rewrite (URLRewrite HTTPRouteFilter)
	MirrorClusters          []string          `json:"mirrorClusters,omitempty"`          // request_mirror_policies cluster names
	RequestHeadersToAdd     []HeaderOperation `json:"requestHeadersToAdd,omitempty"`     // request_headers_to_add
	ResponseHeadersToAdd    []HeaderOperation `json:"responseHeadersToAdd,omitempty"`    // response_headers_to_add
	ResponseHeadersToRemove []string          `json:"responseHeadersToRemove,omitempty"` // response_headers_to_remove
	TypedPerFilterConfig    map[string]any    `json:"typedPerFilterConfig,omitempty"`    // per-route filter config (Phase 2)
	Metadata                map[string]any    `json:"metadata,omitempty"`                // filter_metadata (Phase 4)
}

// RouteRewrite captures path rewrite configuration on a route action.
// Kgateway expresses URLRewrite HTTPRouteFilters as a regex_rewrite in the Envoy route action.
type RouteRewrite struct {
	RegexPattern string `json:"regexPattern"` // regex_rewrite.pattern.regex
	Substitution string `json:"substitution"` // regex_rewrite.substitution
}

// HeaderOperation is a key/value header to add to a request or response.
type HeaderOperation struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RouteMatch describes what traffic a route matches.
type RouteMatch struct {
	Prefix              string            `json:"prefix,omitempty"`
	PathSeparatedPrefix string            `json:"pathSeparatedPrefix,omitempty"` // path_separated_prefix in Envoy
	Path                string            `json:"path,omitempty"`
	Regex               string            `json:"regex,omitempty"` // safe_regex in Envoy
	Headers             []HeaderMatch     `json:"headers,omitempty"`
	QueryParams         []QueryParamMatch `json:"queryParams,omitempty"`
}

// HeaderMatch is a header-based match condition on a route.
type HeaderMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Regex bool   `json:"regex,omitempty"` // true when value is a safe_regex pattern
}

// QueryParamMatch is a query parameter match condition on a route.
type QueryParamMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Regex bool   `json:"regex,omitempty"` // true when value is a safe_regex pattern
}
