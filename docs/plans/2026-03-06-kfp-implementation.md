# kfp — Kgateway Filter Chain Printer: Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a CLI tool that visualizes the Envoy filter chain for a given HTTPRoute on a live Kubernetes cluster, correlating runtime Envoy config back to originating K8S Gateway API resources.

**Architecture:** Sequential pipeline — CLI (cobra) → Resolver (K8S graph walk) → Envoy Fetcher (port-forward + /config_dump) → Correlator (K8S + Envoy merge) → Renderer (bubbletea TUI). The central `RouteGraph` model is the handoff between stages and is designed for future JSON serialization.

**Tech Stack:** Go, cobra, client-go, controller-runtime, gateway-api types, go-control-plane (Envoy proto types), bubbletea, lipgloss, bubbles.

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/kfp/main.go`
- Create: `internal/model/graph.go`

**Step 1: Initialize Go module**

```bash
go mod init github.com/kgateway-dev/kfp
```

**Step 2: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get sigs.k8s.io/controller-runtime@latest
go get sigs.k8s.io/gateway-api@latest
go get k8s.io/client-go@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/envoyproxy/go-control-plane@latest
```

**Step 3: Create the CLI entrypoint**

Create `cmd/kfp/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "kfp",
		Short: "Kgateway filter chain printer",
	}

	route := &cobra.Command{
		Use:   "route <name>",
		Short: "Print the filter chain for an HTTPRoute",
		Args:  cobra.ExactArgs(1),
		RunE:  runRoute,
	}

	route.Flags().StringP("namespace", "n", "default", "Namespace of the HTTPRoute")
	route.Flags().String("context", "", "Kubeconfig context to use (default: current context)")
	route.Flags().Int("rule", -1, "HTTPRoute rule index to inspect (-1 = all rules)")
	route.Flags().Bool("verbose", false, "Show raw Envoy config and match method in expanded panels")

	root.AddCommand(route)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoute(cmd *cobra.Command, args []string) error {
	name := args[0]
	namespace, _ := cmd.Flags().GetString("namespace")
	fmt.Printf("kfp route %s/%s — not yet implemented\n", namespace, name)
	return nil
}
```

**Step 4: Verify it builds and runs**

```bash
go run ./cmd/kfp route my-route -n default
```

Expected output:
```
kfp route default/my-route — not yet implemented
```

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/kfp/main.go
git commit -m "feat: scaffold CLI with cobra route command"
```

---

## Task 2: Data Model

**Files:**
- Create: `internal/model/graph.go`
- Create: `internal/model/graph_test.go`

**Step 1: Write the failing test**

Create `internal/model/graph_test.go`:

```go
package model_test

import (
	"encoding/json"
	"testing"

	"github.com/kgateway-dev/kfp/internal/model"
)

func TestRouteGraphJSONRoundtrip(t *testing.T) {
	graph := model.RouteGraph{
		HTTPRoute: model.K8SRef{Kind: "HTTPRoute", Name: "my-route", Namespace: "default"},
		Rule:      0,
		Gateway: model.GatewayNode{
			Ref: model.K8SRef{Kind: "Gateway", Name: "prod-gw", Namespace: "default"},
			Listener: model.ListenerNode{
				Name:     "https",
				Protocol: "HTTPS",
				Port:     443,
				Chain: model.FilterChain{
					Filters: []model.FilterNode{
						{
							EnvoyName: "envoy.filters.http.jwt_authn",
							Label:     "JWT Auth",
							Details:   []string{"issuer: https://auth.example.com"},
							PolicyRef: &model.K8SRef{Kind: "AuthPolicy", Name: "jwt-policy", Namespace: "default"},
							MatchMethod: model.MatchMetadata,
						},
					},
					Backend: model.BackendNode{
						Refs: []model.BackendRef{
							{
								ServiceRef: model.K8SRef{Kind: "Service", Name: "my-svc", Namespace: "default"},
								Port:       8080,
								Weight:     100,
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got model.RouteGraph
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.HTTPRoute.Name != "my-route" {
		t.Errorf("expected HTTPRoute name 'my-route', got %q", got.HTTPRoute.Name)
	}
	if len(got.Gateway.Listener.Chain.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(got.Gateway.Listener.Chain.Filters))
	}
	if got.Gateway.Listener.Chain.Filters[0].Label != "JWT Auth" {
		t.Errorf("expected filter label 'JWT Auth', got %q", got.Gateway.Listener.Chain.Filters[0].Label)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/model/... -v
```

Expected: FAIL — package does not exist yet.

**Step 3: Implement the model**

Create `internal/model/graph.go`:

```go
package model

// MatchMethod describes how a FilterNode was correlated to a K8S resource.
type MatchMethod int

const (
	// MatchMetadata means kgateway embedded a direct K8S reference in the Envoy config.
	MatchMetadata MatchMethod = iota
	// MatchStructural means correlation via VirtualHost domains, route match config, or cluster names.
	MatchStructural
	// MatchConvention means correlation via filter type and name conventions.
	MatchConvention
)

// RouteGraph is the central artifact produced by the Correlator.
// It is designed for direct JSON serialization to support a future REST API.
type RouteGraph struct {
	HTTPRoute K8SRef      `json:"httpRoute"`
	Rule      int         `json:"rule"` // -1 = all rules
	Gateway   GatewayNode `json:"gateway"`
}

type GatewayNode struct {
	Ref      K8SRef       `json:"ref"`
	Listener ListenerNode `json:"listener"`
}

type ListenerNode struct {
	Name     string      `json:"name"`
	Protocol string      `json:"protocol"` // HTTP, HTTPS, TLS
	Port     int         `json:"port"`
	Chain    FilterChain `json:"chain"`
}

type FilterChain struct {
	Filters []FilterNode `json:"filters"`
	Backend BackendNode  `json:"backend"`
}

type FilterNode struct {
	// Envoy identity
	EnvoyName   string `json:"envoyName"`
	EnvoyConfig any    `json:"envoyConfig,omitempty"` // raw typed config for verbose view

	// K8S origin
	PolicyRef   *K8SRef     `json:"policyRef,omitempty"`
	MatchMethod MatchMethod `json:"matchMethod"`

	// TUI display
	Label   string   `json:"label"`
	Details []string `json:"details,omitempty"`
}

type BackendNode struct {
	Refs []BackendRef `json:"refs"`
}

type BackendRef struct {
	ServiceRef K8SRef `json:"serviceRef"`
	Port       int    `json:"port"`
	Weight     int    `json:"weight"`
}

type K8SRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/model/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/model/graph.go internal/model/graph_test.go
git commit -m "feat: add RouteGraph data model"
```

---

## Task 3: K8S Resolver

**Files:**
- Create: `internal/resolver/resolver.go`
- Create: `internal/resolver/resolver_test.go`

The Resolver walks the K8S resource graph starting from an HTTPRoute and returns the K8S side of the `RouteGraph` (no Envoy data yet — `FilterChain.Filters` will be empty, but Gateway/Listener/Backend are populated).

**Step 1: Write failing tests**

Create `internal/resolver/resolver_test.go`:

```go
package resolver_test

import (
	"context"
	"testing"

	"github.com/kgateway-dev/kfp/internal/model"
	"github.com/kgateway-dev/kfp/internal/resolver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestResolveHTTPRoute_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = gatewayv1.Install(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := resolver.New(c)

	_, err := r.Resolve(context.Background(), "missing-route", "default", -1)
	if err == nil {
		t.Fatal("expected error for missing HTTPRoute, got nil")
	}
}

func TestResolveHTTPRoute_NotAttached(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = gatewayv1.Install(scheme)

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: "prod-gw"},
				},
			},
		},
		// No status.parents — not accepted by any gateway
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	r := resolver.New(c)

	_, err := r.Resolve(context.Background(), "my-route", "default", -1)
	if err == nil {
		t.Fatal("expected error for unattached HTTPRoute, got nil")
	}
}

func TestResolveHTTPRoute_Basic(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = gatewayv1.Install(scheme)

	ns := gatewayv1.Namespace("default")
	port := gatewayv1.PortNumber(8080)
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "kgateway",
			Listeners: []gatewayv1.Listener{
				{Name: "http", Protocol: "HTTP", Port: 80},
			},
		},
	}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: "prod-gw"},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "my-svc",
									Namespace: &ns,
									Port:      &port,
								},
							},
						},
					},
				},
			},
		},
		Status: gatewayv1.HTTPRouteStatus{
			RouteStatus: gatewayv1.RouteStatus{
				Parents: []gatewayv1.RouteParentStatus{
					{
						ParentRef: gatewayv1.ParentReference{Name: "prod-gw"},
						Conditions: []metav1.Condition{
							{Type: "Accepted", Status: "True"},
						},
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gw, route).Build()
	r := resolver.New(c)

	graph, err := r.Resolve(context.Background(), "my-route", "default", -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if graph.HTTPRoute.Name != "my-route" {
		t.Errorf("expected HTTPRoute name 'my-route', got %q", graph.HTTPRoute.Name)
	}
	if graph.Gateway.Ref.Name != "prod-gw" {
		t.Errorf("expected Gateway name 'prod-gw', got %q", graph.Gateway.Ref.Name)
	}
	if len(graph.Gateway.Listener.Chain.Backend.Refs) != 1 {
		t.Errorf("expected 1 backend ref, got %d", len(graph.Gateway.Listener.Chain.Backend.Refs))
	}
	if graph.Gateway.Listener.Chain.Backend.Refs[0].ServiceRef.Name != "my-svc" {
		t.Errorf("expected backend 'my-svc', got %q", graph.Gateway.Listener.Chain.Backend.Refs[0].ServiceRef.Name)
	}
	_ = model.RouteGraph{} // ensure model import is used
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/resolver/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the Resolver**

Create `internal/resolver/resolver.go`:

```go
package resolver

import (
	"context"
	"fmt"

	"github.com/kgateway-dev/kfp/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Resolver walks the K8S Gateway API resource graph and populates
// the K8S side of a RouteGraph (no Envoy data — filters are empty).
type Resolver struct {
	client client.Client
}

func New(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// Resolve fetches an HTTPRoute and walks to its Gateway, listener, and backends.
// rule = -1 means all rules; only the first rule's backends are used when multiple rules exist.
func (r *Resolver) Resolve(ctx context.Context, name, namespace string, rule int) (*model.RouteGraph, error) {
	route := &gatewayv1.HTTPRoute{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, route); err != nil {
		return nil, fmt.Errorf("HTTPRoute %s/%s not found: %w", namespace, name, err)
	}

	// Check that the route is accepted by at least one Gateway
	gatewayName, err := acceptedGateway(route)
	if err != nil {
		return nil, err
	}

	// Determine Gateway namespace (default to HTTPRoute namespace)
	gwNamespace := namespace
	for _, pr := range route.Spec.ParentRefs {
		if string(pr.Name) == gatewayName && pr.Namespace != nil {
			gwNamespace = string(*pr.Namespace)
		}
	}

	gw := &gatewayv1.Gateway{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: gatewayName, Namespace: gwNamespace}, gw); err != nil {
		return nil, fmt.Errorf("Gateway %s/%s not found: %w", gwNamespace, gatewayName, err)
	}

	listener := primaryListener(gw)
	backends := resolveBackends(route, rule)

	graph := &model.RouteGraph{
		HTTPRoute: model.K8SRef{Kind: "HTTPRoute", Name: route.Name, Namespace: route.Namespace},
		Rule:      rule,
		Gateway: model.GatewayNode{
			Ref: model.K8SRef{Kind: "Gateway", Name: gw.Name, Namespace: gw.Namespace},
			Listener: model.ListenerNode{
				Name:     string(listener.Name),
				Protocol: string(listener.Protocol),
				Port:     int(listener.Port),
				Chain: model.FilterChain{
					Filters: []model.FilterNode{}, // populated by Correlator
					Backend: model.BackendNode{Refs: backends},
				},
			},
		},
	}

	return graph, nil
}

// acceptedGateway returns the name of the first Gateway that has accepted this route.
func acceptedGateway(route *gatewayv1.HTTPRoute) (string, error) {
	for _, parent := range route.Status.Parents {
		for _, cond := range parent.Conditions {
			if cond.Type == string(metav1.StatusReasonMethodNotAllowed) {
				continue
			}
			if cond.Type == "Accepted" && cond.Status == "True" {
				return string(parent.ParentRef.Name), nil
			}
		}
	}
	return "", fmt.Errorf("HTTPRoute is not accepted by any Gateway — check route status and parentRefs")
}

// primaryListener returns the first listener on the Gateway.
// In practice kfp could be extended to let the user pick a listener.
func primaryListener(gw *gatewayv1.Gateway) gatewayv1.Listener {
	if len(gw.Spec.Listeners) == 0 {
		return gatewayv1.Listener{Name: "unknown"}
	}
	return gw.Spec.Listeners[0]
}

// resolveBackends collects backend refs from the HTTPRoute rules.
// If rule >= 0, only that rule's backends are collected.
func resolveBackends(route *gatewayv1.HTTPRoute, rule int) []model.BackendRef {
	var refs []model.BackendRef
	for i, r := range route.Spec.Rules {
		if rule >= 0 && i != rule {
			continue
		}
		for _, br := range r.BackendRefs {
			ns := route.Namespace
			if br.Namespace != nil {
				ns = string(*br.Namespace)
			}
			port := 0
			if br.Port != nil {
				port = int(*br.Port)
			}
			weight := 1
			if br.Weight != nil {
				weight = int(*br.Weight)
			}
			refs = append(refs, model.BackendRef{
				ServiceRef: model.K8SRef{
					Kind:      "Service",
					Name:      string(br.Name),
					Namespace: ns,
				},
				Port:   port,
				Weight: weight,
			})
		}
	}
	return refs
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/resolver/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/resolver/resolver.go internal/resolver/resolver_test.go
git commit -m "feat: add K8S resource graph resolver"
```

---

## Task 4: Envoy Fetcher

**Files:**
- Create: `internal/envoy/portforward.go`
- Create: `internal/envoy/client.go`
- Create: `internal/envoy/client_test.go`

The Envoy fetcher locates the gateway-proxy pod for a given Gateway, opens a port-forward to the Envoy admin port, fetches `/config_dump`, and returns the raw JSON bytes. Port-forwarding uses the Kubernetes SPDY client from `k8s.io/client-go`.

**Step 1: Write failing tests**

Create `internal/envoy/client_test.go`:

```go
package envoy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kgateway-dev/kfp/internal/envoy"
)

func TestFetchConfigDump_Success(t *testing.T) {
	// Simulate an Envoy admin endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config_dump" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Minimal valid config_dump structure
		_, _ = w.Write([]byte(`{"configs": []}`))
	}))
	defer srv.Close()

	client := envoy.NewAdminClient(srv.URL)
	data, err := client.ConfigDump()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty config dump")
	}
}

func TestFetchConfigDump_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := envoy.NewAdminClient(srv.URL)
	_, err := client.ConfigDump()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/envoy/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the admin client**

Create `internal/envoy/client.go`:

```go
package envoy

import (
	"fmt"
	"io"
	"net/http"
)

// AdminClient fetches data from the Envoy admin API.
type AdminClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAdminClient returns an AdminClient targeting the given base URL (e.g. "http://localhost:19000").
func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// ConfigDump fetches /config_dump and returns the raw JSON bytes.
func (c *AdminClient) ConfigDump() ([]byte, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/config_dump")
	if err != nil {
		return nil, fmt.Errorf("GET /config_dump failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Envoy admin returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading config_dump response: %w", err)
	}
	return data, nil
}
```

**Step 4: Implement the port-forwarder**

Create `internal/envoy/portforward.go`:

```go
package envoy

import (
	"context"
	"fmt"
	"net"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const envoyAdminPort = 19000

// PortForwardResult holds the local address and a stop function to tear down the forward.
type PortForwardResult struct {
	LocalAddr string
	Stop      func()
}

// FindAndPortForward locates a ready gateway-proxy pod for the given Gateway
// and opens a port-forward to its Envoy admin port.
// The returned Stop function MUST be called when done.
func FindAndPortForward(ctx context.Context, cfg *rest.Config, gatewayName, gatewayNamespace string) (*PortForwardResult, error) {
	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	podName, err := findGatewayProxyPod(ctx, kc, gatewayName, gatewayNamespace)
	if err != nil {
		return nil, err
	}

	// Find a free local port
	localPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding free local port: %w", err)
	}

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY transport: %w", err)
	}

	url := kc.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(gatewayNamespace).
		Name(podName).
		SubResource("portforward").URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	ports := []string{fmt.Sprintf("%d:%d", localPort, envoyAdminPort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	go func() {
		errCh <- fw.ForwardPorts()
	}()

	// Wait for port-forward to be ready or fail
	select {
	case <-readyCh:
	case err := <-errCh:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	}

	return &PortForwardResult{
		LocalAddr: fmt.Sprintf("http://localhost:%d", localPort),
		Stop:      func() { close(stopCh) },
	}, nil
}

// findGatewayProxyPod finds the first ready pod for a kgateway Gateway.
// kgateway labels gateway proxy pods with gateway.networking.k8s.io/gateway-name.
func findGatewayProxyPod(ctx context.Context, kc kubernetes.Interface, gatewayName, namespace string) (string, error) {
	selector := fmt.Sprintf("gateway.networking.k8s.io/gateway-name=%s", gatewayName)
	pods, err := kc.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("listing gateway-proxy pods: %w", err)
	}

	for _, pod := range pods.Items {
		if isPodReady(&pod) {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf(
		"no ready gateway-proxy pod found for Gateway %s/%s (selector: %s)\n"+
			"Hint: ensure the gateway pod is running and you have 'get pods' RBAC permission in namespace %s",
		namespace, gatewayName, selector, namespace,
	)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/envoy/... -v
```

Expected: PASS (portforward.go has no unit tests — it requires a live cluster; tested in integration)

**Step 6: Commit**

```bash
git add internal/envoy/client.go internal/envoy/portforward.go internal/envoy/client_test.go
git commit -m "feat: add Envoy admin client and port-forwarder"
```

---

## Task 5: Correlator

**Files:**
- Create: `internal/correlator/correlator.go`
- Create: `internal/correlator/correlator_test.go`
- Create: `internal/correlator/testdata/config_dump.json`

The Correlator takes the K8S `RouteGraph` (from the Resolver) and the raw Envoy config dump JSON, and produces a complete `RouteGraph` with `FilterChain.Filters` populated and correlated back to K8S resources.

**Step 1: Create test fixture**

Create `internal/correlator/testdata/config_dump.json` with a minimal realistic Envoy config dump that includes a listener with HTTP filters, a route config, and clusters. This fixture represents what kgateway would produce for a route with JWT auth:

```json
{
  "configs": [
    {
      "@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
      "dynamic_listeners": [
        {
          "name": "0.0.0.0_443",
          "active_state": {
            "listener": {
              "@type": "type.googleapis.com/envoy.config.listener.v3.Listener",
              "name": "0.0.0.0_443",
              "filter_chains": [
                {
                  "filters": [
                    {
                      "name": "envoy.filters.network.http_connection_manager",
                      "typed_config": {
                        "@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
                        "http_filters": [
                          {
                            "name": "envoy.filters.http.jwt_authn",
                            "typed_config": {
                              "@type": "type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication",
                              "providers": {
                                "my-jwt": {
                                  "issuer": "https://auth.example.com",
                                  "audiences": ["my-api"]
                                }
                              }
                            }
                          },
                          {
                            "name": "envoy.filters.http.router",
                            "typed_config": {
                              "@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
                            }
                          }
                        ],
                        "virtual_hosts": [
                          {
                            "name": "my-route_default",
                            "domains": ["api.example.com"],
                            "routes": [
                              {
                                "match": {"prefix": "/"},
                                "route": {"cluster": "default_my-svc_8080"},
                                "metadata": {
                                  "filter_metadata": {
                                    "io.solo.kgateway": {
                                      "policy_ref": {
                                        "kind": "AuthPolicy",
                                        "name": "jwt-policy",
                                        "namespace": "default"
                                      }
                                    }
                                  }
                                }
                              }
                            ]
                          }
                        ]
                      }
                    }
                  ]
                }
              ]
            }
          }
        }
      ]
    }
  ]
}
```

**Step 2: Write failing tests**

Create `internal/correlator/correlator_test.go`:

```go
package correlator_test

import (
	"os"
	"testing"

	"github.com/kgateway-dev/kfp/internal/correlator"
	"github.com/kgateway-dev/kfp/internal/model"
)

func TestCorrelate_JWTFilter(t *testing.T) {
	configDump, err := os.ReadFile("testdata/config_dump.json")
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}

	k8sGraph := &model.RouteGraph{
		HTTPRoute: model.K8SRef{Kind: "HTTPRoute", Name: "my-route", Namespace: "default"},
		Rule:      -1,
		Gateway: model.GatewayNode{
			Ref: model.K8SRef{Kind: "Gateway", Name: "prod-gw", Namespace: "default"},
			Listener: model.ListenerNode{
				Name:     "https",
				Protocol: "HTTPS",
				Port:     443,
				Chain: model.FilterChain{
					Filters: []model.FilterNode{},
					Backend: model.BackendNode{
						Refs: []model.BackendRef{
							{
								ServiceRef: model.K8SRef{Kind: "Service", Name: "my-svc", Namespace: "default"},
								Port:       8080,
								Weight:     1,
							},
						},
					},
				},
			},
		},
	}

	c := correlator.New()
	graph, err := c.Correlate(k8sGraph, configDump)
	if err != nil {
		t.Fatalf("correlate failed: %v", err)
	}

	filters := graph.Gateway.Listener.Chain.Filters
	// Router filter should be excluded (it's internal to Envoy)
	if len(filters) == 0 {
		t.Fatal("expected at least one filter")
	}

	jwtFilter := filters[0]
	if jwtFilter.EnvoyName != "envoy.filters.http.jwt_authn" {
		t.Errorf("expected jwt filter, got %q", jwtFilter.EnvoyName)
	}
	if jwtFilter.Label != "JWT Auth" {
		t.Errorf("expected label 'JWT Auth', got %q", jwtFilter.Label)
	}
	if jwtFilter.PolicyRef == nil {
		t.Fatal("expected PolicyRef to be set")
	}
	if jwtFilter.PolicyRef.Name != "jwt-policy" {
		t.Errorf("expected policy 'jwt-policy', got %q", jwtFilter.PolicyRef.Name)
	}
	if jwtFilter.MatchMethod != model.MatchMetadata {
		t.Errorf("expected MatchMetadata, got %v", jwtFilter.MatchMethod)
	}
}
```

**Step 3: Run tests to verify they fail**

```bash
go test ./internal/correlator/... -v
```

Expected: FAIL — package does not exist.

**Step 4: Implement the Correlator**

Create `internal/correlator/correlator.go`:

```go
package correlator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kgateway-dev/kfp/internal/model"
)

// knownFilters maps Envoy filter names to human-readable labels and detail extractors.
var knownFilters = map[string]filterMeta{
	"envoy.filters.http.jwt_authn":         {label: "JWT Auth"},
	"envoy.filters.http.ext_authz":         {label: "External Auth"},
	"envoy.filters.http.ratelimit":         {label: "Rate Limiting"},
	"envoy.filters.http.lua":               {label: "Lua Transform"},
	"envoy.filters.http.fault":             {label: "Fault Injection"},
	"envoy.filters.http.cors":              {label: "CORS"},
	"envoy.filters.http.header_to_metadata": {label: "Header Transform"},
	"envoy.filters.http.wasm":              {label: "WASM"},
}

// filtersToSkip are internal Envoy filters not worth surfacing to users.
var filtersToSkip = map[string]bool{
	"envoy.filters.http.router": true,
}

type filterMeta struct {
	label string
}

// Correlator merges K8S RouteGraph with Envoy config dump data.
type Correlator struct{}

func New() *Correlator {
	return &Correlator{}
}

// Correlate takes the K8S-side RouteGraph (from Resolver) and the raw Envoy
// /config_dump JSON, and returns a complete RouteGraph with filters correlated
// back to their K8S origins.
func (c *Correlator) Correlate(k8s *model.RouteGraph, configDump []byte) (*model.RouteGraph, error) {
	dump, err := parseConfigDump(configDump)
	if err != nil {
		return nil, fmt.Errorf("parsing config dump: %w", err)
	}

	filters, err := extractFilters(dump, k8s)
	if err != nil {
		return nil, fmt.Errorf("extracting filters: %w", err)
	}

	result := *k8s // shallow copy
	result.Gateway.Listener.Chain.Filters = filters
	return &result, nil
}

// --- Config dump parsing (raw JSON structs, avoiding full proto dependency for now) ---

type configDump struct {
	Configs []json.RawMessage `json:"configs"`
}

type listenersConfigDump struct {
	DynamicListeners []dynamicListener `json:"dynamic_listeners"`
}

type dynamicListener struct {
	Name        string      `json:"name"`
	ActiveState activeState `json:"active_state"`
}

type activeState struct {
	Listener listenerConfig `json:"listener"`
}

type listenerConfig struct {
	FilterChains []filterChainConfig `json:"filter_chains"`
}

type filterChainConfig struct {
	Filters []networkFilter `json:"filters"`
}

type networkFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

type hcmConfig struct {
	HTTPFilters  []httpFilter  `json:"http_filters"`
	VirtualHosts []virtualHost `json:"virtual_hosts"`
}

type httpFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

type virtualHost struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Routes  []envoyRoute `json:"routes"`
}

type envoyRoute struct {
	Metadata *routeMetadata `json:"metadata,omitempty"`
}

type routeMetadata struct {
	FilterMetadata map[string]json.RawMessage `json:"filter_metadata"`
}

type kgatewayMeta struct {
	PolicyRef *struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"policy_ref"`
}

func parseConfigDump(data []byte) (*configDump, error) {
	var dump configDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, err
	}
	return &dump, nil
}

// extractFilters walks the config dump and builds the filter list,
// using the layered correlation strategy.
func extractFilters(dump *configDump, k8s *model.RouteGraph) ([]model.FilterNode, error) {
	// Extract kgateway metadata from route entries (metadata-based correlation)
	policyRefs := extractMetadataPolicyRefs(dump)

	// Find the HCM http_filters list from the first matching listener
	httpFilters := extractHTTPFilters(dump)

	var nodes []model.FilterNode
	for _, hf := range httpFilters {
		if filtersToSkip[hf.Name] {
			continue
		}

		node := model.FilterNode{
			EnvoyName:   hf.Name,
			MatchMethod: model.MatchConvention,
		}

		// Apply human-readable label
		if meta, ok := knownFilters[hf.Name]; ok {
			node.Label = meta.label
		} else {
			node.Label = hf.Name // fallback: show raw filter name
		}

		// Layer 1: metadata-based policy ref
		if ref, ok := policyRefs[hf.Name]; ok {
			node.PolicyRef = ref
			node.MatchMethod = model.MatchMetadata
		}

		// Layer 2: structural — try to match by cluster name to backend service
		// (for backend-level filters this would be extended further)

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// extractMetadataPolicyRefs pulls kgateway policy_ref metadata from route entries.
// Returns a map of envoy filter name → K8SRef.
func extractMetadataPolicyRefs(dump *configDump) map[string]*model.K8SRef {
	refs := map[string]*model.K8SRef{}

	for _, raw := range dump.Configs {
		var ld listenersConfigDump
		if err := json.Unmarshal(raw, &ld); err != nil {
			continue
		}
		for _, dl := range ld.DynamicListeners {
			for _, fc := range dl.ActiveState.Listener.FilterChains {
				for _, nf := range fc.Filters {
					if nf.Name != "envoy.filters.network.http_connection_manager" {
						continue
					}
					var hcm hcmConfig
					if err := json.Unmarshal(nf.TypedConfig, &hcm); err != nil {
						continue
					}
					for _, vh := range hcm.VirtualHosts {
						for _, route := range vh.Routes {
							if route.Metadata == nil {
								continue
							}
							rawMeta, ok := route.Metadata.FilterMetadata["io.solo.kgateway"]
							if !ok {
								continue
							}
							var km kgatewayMeta
							if err := json.Unmarshal(rawMeta, &km); err != nil {
								continue
							}
							if km.PolicyRef != nil {
								// Map the policy type to its associated filter name
								filterName := policyKindToFilterName(km.PolicyRef.Kind)
								refs[filterName] = &model.K8SRef{
									Kind:      km.PolicyRef.Kind,
									Name:      km.PolicyRef.Name,
									Namespace: km.PolicyRef.Namespace,
								}
							}
						}
					}
				}
			}
		}
	}
	return refs
}

// extractHTTPFilters returns the http_filters list from the first HCM found in the dump.
func extractHTTPFilters(dump *configDump) []httpFilter {
	for _, raw := range dump.Configs {
		var ld listenersConfigDump
		if err := json.Unmarshal(raw, &ld); err != nil {
			continue
		}
		for _, dl := range ld.DynamicListeners {
			for _, fc := range dl.ActiveState.Listener.FilterChains {
				for _, nf := range fc.Filters {
					if nf.Name != "envoy.filters.network.http_connection_manager" {
						continue
					}
					var hcm hcmConfig
					if err := json.Unmarshal(nf.TypedConfig, &hcm); err != nil {
						continue
					}
					return hcm.HTTPFilters
				}
			}
		}
	}
	return nil
}

// policyKindToFilterName maps a K8S policy kind to the Envoy filter it configures.
func policyKindToFilterName(kind string) string {
	switch strings.ToLower(kind) {
	case "authpolicy":
		return "envoy.filters.http.jwt_authn"
	case "ratelimitpolicy":
		return "envoy.filters.http.ratelimit"
	case "trafficpolicy":
		return "envoy.filters.http.header_to_metadata"
	default:
		return kind
	}
}
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/correlator/... -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/correlator/correlator.go internal/correlator/correlator_test.go internal/correlator/testdata/config_dump.json
git commit -m "feat: add filter chain correlator with layered K8S/Envoy matching"
```

---

## Task 6: Renderer (TUI)

**Files:**
- Create: `internal/renderer/renderer.go`
- Create: `internal/renderer/renderer_test.go`

The Renderer takes a `RouteGraph` and renders it as a bubbletea TUI with lipgloss-styled panels. Filters are listed in order; arrow keys navigate; Enter expands a filter to show details.

**Step 1: Write failing test (static render)**

Create `internal/renderer/renderer_test.go`:

```go
package renderer_test

import (
	"strings"
	"testing"

	"github.com/kgateway-dev/kfp/internal/model"
	"github.com/kgateway-dev/kfp/internal/renderer"
)

func TestRenderSummary_ContainsKeyInfo(t *testing.T) {
	graph := &model.RouteGraph{
		HTTPRoute: model.K8SRef{Kind: "HTTPRoute", Name: "my-route", Namespace: "default"},
		Rule:      -1,
		Gateway: model.GatewayNode{
			Ref: model.K8SRef{Kind: "Gateway", Name: "prod-gw", Namespace: "default"},
			Listener: model.ListenerNode{
				Name:     "https",
				Protocol: "HTTPS",
				Port:     443,
				Chain: model.FilterChain{
					Filters: []model.FilterNode{
						{
							EnvoyName:   "envoy.filters.http.jwt_authn",
							Label:       "JWT Auth",
							Details:     []string{"issuer: https://auth.example.com"},
							PolicyRef:   &model.K8SRef{Kind: "AuthPolicy", Name: "jwt-policy", Namespace: "default"},
							MatchMethod: model.MatchMetadata,
						},
					},
					Backend: model.BackendNode{
						Refs: []model.BackendRef{
							{
								ServiceRef: model.K8SRef{Kind: "Service", Name: "my-svc", Namespace: "default"},
								Port:       8080,
								Weight:     100,
							},
						},
					},
				},
			},
		},
	}

	output := renderer.RenderSummary(graph)

	checks := []string{
		"my-route",
		"prod-gw",
		"HTTPS",
		"JWT Auth",
		"jwt-policy",
		"my-svc",
		"8080",
	}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/renderer/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the renderer**

Create `internal/renderer/renderer.go`:

```go
package renderer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kgateway-dev/kfp/internal/model"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")). // bright blue
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	filterStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			Padding(0, 1)

	selectedFilterStyle = filterStyle.Copy().
				Foreground(lipgloss.Color("11")). // yellow
				Bold(true)

	backendStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // green
			Bold(true).
			Padding(0, 1)

	policyRefStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")). // dark gray
			Italic(true)

	labelStyle = lipgloss.NewStyle().
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // red
			Bold(true)
)

// RenderSummary renders the RouteGraph as a plain string (no interactive TUI).
// Used for testing and for simple non-TTY output.
func RenderSummary(graph *model.RouteGraph) string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf(
		"HTTPRoute: %s  |  namespace: %s\nGateway: %s  |  Listener: %s:%d (%s)",
		graph.HTTPRoute.Name,
		graph.HTTPRoute.Namespace,
		graph.Gateway.Ref.Name,
		graph.Gateway.Listener.Name,
		graph.Gateway.Listener.Port,
		graph.Gateway.Listener.Protocol,
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n\n")

	// Filter chain
	b.WriteString("  FILTER CHAIN\n")
	for i, f := range graph.Gateway.Listener.Chain.Filters {
		b.WriteString(renderFilter(i+1, f, false))
		b.WriteString("\n")
	}

	// Backend
	b.WriteString(renderBackend(graph.Gateway.Listener.Chain.Backend))

	return b.String()
}

func renderFilter(pos int, f model.FilterNode, selected bool) string {
	style := filterStyle
	if selected {
		style = selectedFilterStyle
	}

	ref := ""
	if f.PolicyRef != nil {
		ref = policyRefStyle.Render(fmt.Sprintf("[%s: %s]", f.PolicyRef.Kind, f.PolicyRef.Name))
	} else {
		ref = warningStyle.Render("[no K8S policy found]")
	}

	label := labelStyle.Render(fmt.Sprintf("%d  %s", pos, f.Label))
	line := fmt.Sprintf("%s  %s", label, ref)

	if len(f.Details) > 0 {
		line += "\n   " + strings.Join(f.Details, "  |  ")
	}

	return style.Render(line)
}

func renderBackend(backend model.BackendNode) string {
	var parts []string
	for _, ref := range backend.Refs {
		parts = append(parts, fmt.Sprintf(
			"▶ Backend: %s:%d  (weight: %d%%)",
			ref.ServiceRef.Name, ref.Port, ref.Weight,
		))
	}
	return backendStyle.Render(strings.Join(parts, "\n"))
}

// --- Interactive TUI (bubbletea model) ---

// Model is the bubbletea application model for interactive TUI mode.
type Model struct {
	graph    *model.RouteGraph
	selected int  // currently selected filter index
	expanded bool // whether the selected filter is expanded
	verbose  bool
}

func NewModel(graph *model.RouteGraph, verbose bool) Model {
	return Model{graph: graph, verbose: verbose}
}

func (m Model) Init() interface{} { return nil }

func (m Model) Update(msg interface{}) (Model, interface{}) {
	// Key handling is wired up in Run() via bubbletea — stubbed here for structure
	return m, nil
}

func (m Model) View() string {
	return RenderSummary(m.graph)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/renderer/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/renderer/renderer.go internal/renderer/renderer_test.go
git commit -m "feat: add lipgloss TUI renderer"
```

---

## Task 7: Wire the Pipeline in the CLI

**Files:**
- Modify: `cmd/kfp/main.go`
- Create: `internal/k8sclient/client.go`

**Step 1: Create the K8S client factory**

Create `internal/k8sclient/client.go`:

```go
package k8sclient

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
}

// Build returns a controller-runtime client using the given kubeconfig context.
// If contextName is empty, the current context is used.
func Build(contextName string) (client.Client, *rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("creating K8S client: %w", err)
	}

	return c, cfg, nil
}
```

**Step 2: Wire everything in runRoute**

Replace `runRoute` in `cmd/kfp/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kgateway-dev/kfp/internal/correlator"
	"github.com/kgateway-dev/kfp/internal/envoy"
	"github.com/kgateway-dev/kfp/internal/k8sclient"
	"github.com/kgateway-dev/kfp/internal/renderer"
	"github.com/kgateway-dev/kfp/internal/resolver"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "kfp",
		Short: "Kgateway filter chain printer",
	}

	route := &cobra.Command{
		Use:   "route <name>",
		Short: "Print the filter chain for an HTTPRoute",
		Args:  cobra.ExactArgs(1),
		RunE:  runRoute,
	}

	route.Flags().StringP("namespace", "n", "default", "Namespace of the HTTPRoute")
	route.Flags().String("context", "", "Kubeconfig context to use (default: current context)")
	route.Flags().Int("rule", -1, "HTTPRoute rule index to inspect (-1 = all rules)")
	route.Flags().Bool("verbose", false, "Show raw Envoy config and match method in expanded panels")

	root.AddCommand(route)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoute(cmd *cobra.Command, args []string) error {
	name := args[0]
	namespace, _ := cmd.Flags().GetString("namespace")
	contextName, _ := cmd.Flags().GetString("context")
	rule, _ := cmd.Flags().GetInt("rule")
	verbose, _ := cmd.Flags().GetBool("verbose")

	ctx := context.Background()

	// Build K8S client
	k8sClient, restCfg, err := k8sclient.Build(contextName)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	// Resolve K8S resource graph
	res := resolver.New(k8sClient)
	k8sGraph, err := res.Resolve(ctx, name, namespace, rule)
	if err != nil {
		return err
	}

	// Port-forward to Envoy admin and fetch config dump
	fmt.Fprintln(os.Stderr, "Connecting to Envoy admin API...")
	pf, err := envoy.FindAndPortForward(ctx, restCfg, k8sGraph.Gateway.Ref.Name, k8sGraph.Gateway.Ref.Namespace)
	if err != nil {
		return fmt.Errorf("cannot reach Envoy admin API: %w\nHint: ensure you have 'get pods' and 'create pods/portforward' RBAC in namespace %s", err, k8sGraph.Gateway.Ref.Namespace)
	}
	defer pf.Stop()

	adminClient := envoy.NewAdminClient(pf.LocalAddr)
	configDump, err := adminClient.ConfigDump()
	if err != nil {
		return fmt.Errorf("fetching Envoy config dump: %w", err)
	}

	// Correlate
	c := correlator.New()
	graph, err := c.Correlate(k8sGraph, configDump)
	if err != nil {
		return fmt.Errorf("correlating K8S and Envoy config: %w", err)
	}

	// Render
	m := renderer.NewModel(graph, verbose)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("rendering TUI: %w", err)
	}

	return nil
}
```

**Step 3: Implement bubbletea interface on Model**

The `renderer.Model` needs to implement `tea.Model`. Update `internal/renderer/renderer.go` to make `Model` implement `Init()`, `Update()`, and `View()` with proper bubbletea types:

```go
// Add these to the Model methods in renderer.go (replace the stubbed versions):

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				m.expanded = false
			}
		case "down", "j":
			if m.selected < len(m.graph.Gateway.Listener.Chain.Filters)-1 {
				m.selected++
				m.expanded = false
			}
		case "enter":
			m.expanded = !m.expanded
		}
	}
	return m, nil
}

func (m Model) View() string {
	return RenderSummary(m.graph)
}
```

Also add the import: `tea "github.com/charmbracelet/bubbletea"` to `renderer.go`.

**Step 4: Build to verify it compiles**

```bash
go build ./...
```

Expected: no errors.

**Step 5: Run all tests**

```bash
go test ./... -v
```

Expected: all PASS.

**Step 6: Commit**

```bash
git add cmd/kfp/main.go internal/k8sclient/client.go internal/renderer/renderer.go
git commit -m "feat: wire pipeline — resolver, envoy fetcher, correlator, renderer"
```

---

## Task 8: Manual Integration Test

No automated test for this — requires a live cluster with kgateway installed.

**Step 1: Build the binary**

```bash
go build -o kfp ./cmd/kfp
```

**Step 2: Apply a test HTTPRoute to your cluster**

Use any existing kgateway test setup, or apply a minimal HTTPRoute:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: test-route
  namespace: default
spec:
  parentRefs:
    - name: prod-gw
  rules:
    - backendRefs:
        - name: my-svc
          port: 8080
```

**Step 3: Run kfp**

```bash
./kfp route test-route -n default
```

Expected: TUI renders showing the filter chain.

**Step 4: Verify error case**

```bash
./kfp route nonexistent-route -n default
```

Expected: clear error message, non-zero exit code.

---

## Task 9: README

**Files:**
- Create: `README.md`

```markdown
# kfp — Kgateway Filter Chain Printer

Visualizes the Envoy filter chain for a Kubernetes HTTPRoute managed by [Kgateway](https://github.com/kgateway-dev/kgateway).

## Installation

```bash
go install github.com/kgateway-dev/kfp/cmd/kfp@latest
```

## Usage

```bash
# Inspect all rules of an HTTPRoute
kfp route <name> -n <namespace>

# Pin to a specific rule
kfp route <name> -n <namespace> --rule 0

# Override kubeconfig context
kfp route <name> -n <namespace> --context my-cluster

# Show raw Envoy config in expanded panels
kfp route <name> -n <namespace> --verbose
```

## Requirements

- A running Kubernetes cluster with Kgateway installed
- `get pods` and `create pods/portforward` RBAC in the Gateway namespace
- Your kubeconfig pointing at the target cluster

## Navigation

| Key | Action |
|-----|--------|
| `j`/`↓` | Next filter |
| `k`/`↑` | Previous filter |
| `Enter` | Expand/collapse filter detail |
| `q`/`Esc` | Quit |
```

**Step 1: Commit**

```bash
git add README.md
git commit -m "docs: add README with usage instructions"
```
