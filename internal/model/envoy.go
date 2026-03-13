package model

// EnvoySnapshot is the complete parsed Envoy configuration.
// Built by joining data from ListenersConfigDump, RoutesConfigDump, and ClustersConfigDump.
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
	TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config (for Phase 2)
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
	Name                    string           `json:"name"`
	Match                   RouteMatch       `json:"match"`
	Cluster                 string           `json:"cluster"`                        // backend cluster name
	MirrorClusters          []string         `json:"mirrorClusters,omitempty"`       // request_mirror_policies cluster names
	RequestHeadersToAdd     []HeaderOperation `json:"requestHeadersToAdd,omitempty"`  // request_headers_to_add
	ResponseHeadersToAdd    []HeaderOperation `json:"responseHeadersToAdd,omitempty"` // response_headers_to_add
	ResponseHeadersToRemove []string         `json:"responseHeadersToRemove,omitempty"` // response_headers_to_remove
	TypedPerFilterConfig    map[string]any   `json:"typedPerFilterConfig,omitempty"` // per-route filter config (Phase 2)
	Metadata                map[string]any   `json:"metadata,omitempty"`             // filter_metadata (Phase 4)
}

// HeaderOperation is a key/value header to add to a request or response.
type HeaderOperation struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RouteMatch describes what traffic a route matches.
type RouteMatch struct {
	Prefix  string        `json:"prefix,omitempty"`
	Path    string        `json:"path,omitempty"`
	Regex   string        `json:"regex,omitempty"`
	Headers []HeaderMatch `json:"headers,omitempty"`
}

// HeaderMatch is a header-based match condition on a route.
type HeaderMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
