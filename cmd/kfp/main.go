// Command kfp (Kgateway Filter-chain Printer) visualizes the Envoy filter chain
// configuration for a Kubernetes Gateway managed by kgateway.
//
// Usage:
//
//	kfp dump --file <path>                                                                     # parse a local config_dump JSON file
//	kfp dump --gateway <name> -n <ns>                                                          # live fetch via port-forward to gateway-proxy pod
//	kfp dump --deployment <name> -n <ns>                                                       # live fetch via port-forward to any pod in Deployment
//	kfp dump --gateway <name> -n <ns> --context ctx                                            # same, with an explicit kubeconfig context
//	kfp dump --file <path> --httproute <name> --httproute-namespace <ns>                       # filter by HTTPRoute
//	kfp dump --file <path> --httproute <name> --httproute-namespace <ns> --rule 0              # filter by rule index
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DuncanDoyle/kfp/internal/envoy"
	"github.com/DuncanDoyle/kfp/internal/filter"
	"github.com/DuncanDoyle/kfp/internal/parser"
	"github.com/DuncanDoyle/kfp/internal/renderer"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "kfp",
		Short: "Kgateway filter chain printer — visualize Envoy config",
	}

	dump := &cobra.Command{
		Use:   "dump",
		Short: "Dump and visualize the Envoy filter chain configuration",
		RunE:  runDump,
	}

	// Config source — exactly one must be provided.
	dump.Flags().String("file", "", "Path to an Envoy config_dump JSON file")
	dump.Flags().String("gateway", "", "Gateway name (port-forward to gateway-proxy pod via gateway label)")
	dump.Flags().String("deployment", "", "Deployment name (port-forward to any ready pod in the Deployment)")
	dump.Flags().StringP("namespace", "n", "default", "Namespace (used with --gateway or --deployment)")
	dump.Flags().String("context", "", "Kubeconfig context (used with --gateway or --deployment, default: current context)")

	// HTTPRoute filter — narrows the output to routes belonging to one HTTPRoute.
	// --httproute-namespace is intentionally separate from -n/--namespace because
	// the HTTPRoute namespace can differ from the Gateway namespace.
	dump.Flags().String("httproute", "", "Filter output to routes belonging to this HTTPRoute name")
	dump.Flags().String("httproute-namespace", "", "Namespace of the HTTPRoute (required with --httproute; may differ from the Gateway namespace)")
	dump.Flags().Int("rule", -1, "Zero-based rule index within the HTTPRoute (-1 = all rules, used with --httproute)")

	root.AddCommand(dump)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runDump is the cobra RunE handler for the "dump" subcommand.
// It reads the config dump (from a file or live via port-forward), parses it,
// applies any HTTPRoute filter, prints warnings to stderr, then renders the
// snapshot to stdout.
func runDump(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	gateway, _ := cmd.Flags().GetString("gateway")
	deployment, _ := cmd.Flags().GetString("deployment")
	namespace, _ := cmd.Flags().GetString("namespace")
	kubeContext, _ := cmd.Flags().GetString("context")

	httprouteName, _ := cmd.Flags().GetString("httproute")
	httprouteNamespace, _ := cmd.Flags().GetString("httproute-namespace")
	ruleIndex, _ := cmd.Flags().GetInt("rule")

	// Exactly one source must be specified; --gateway and --deployment are mutually exclusive.
	if file == "" && gateway == "" && deployment == "" {
		return fmt.Errorf("specify one of --file <path>, --gateway <name>, or --deployment <name>")
	}
	if file != "" && (gateway != "" || deployment != "") {
		return fmt.Errorf("--file cannot be combined with --gateway or --deployment")
	}
	if gateway != "" && deployment != "" {
		return fmt.Errorf("--gateway and --deployment are mutually exclusive")
	}

	// Validate HTTPRoute filter flags.
	// --httproute and --httproute-namespace must always be used together because
	// two HTTPRoutes with the same name in different namespaces are distinct resources.
	if httprouteName != "" && httprouteNamespace == "" {
		return fmt.Errorf("--httproute-namespace is required when --httproute is set")
	}
	if httprouteNamespace != "" && httprouteName == "" {
		return fmt.Errorf("--httproute-namespace requires --httproute")
	}
	if ruleIndex < -1 {
		return fmt.Errorf("--rule must be >= 0 (use -1 for all rules, which is the default)")
	}
	if ruleIndex >= 0 && httprouteName == "" {
		return fmt.Errorf("--rule requires --httproute")
	}

	// Fetch the raw config dump bytes.
	var data []byte
	var err error

	if file != "" {
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Connecting to Envoy admin API...")
		ctx := context.Background()

		var pf *envoy.PortForwardResult
		if gateway != "" {
			pf, err = envoy.PortForwardToGateway(ctx, gateway, namespace, kubeContext)
		} else {
			pf, err = envoy.PortForwardToDeployment(ctx, deployment, namespace, kubeContext)
		}
		if err != nil {
			return fmt.Errorf("cannot reach Envoy admin API: %w", err)
		}
		defer pf.Stop()

		data, err = envoy.FetchConfigDump(pf.LocalAddr)
		if err != nil {
			return fmt.Errorf("fetching config dump: %w", err)
		}
	}

	// Parse the config dump into an EnvoySnapshot.
	result, err := parser.Parse(data)
	if err != nil {
		return err
	}

	// Print any non-fatal parse warnings to stderr so users know if sections were skipped.
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	// Apply the HTTPRoute filter when requested.
	snapshot := result.Snapshot
	if httprouteName != "" {
		snapshot = filter.Filter(snapshot, filter.FilterOptions{
			HTTPRouteName:      httprouteName,
			HTTPRouteNamespace: httprouteNamespace,
			RuleIndex:          ruleIndex,
		})
		if len(snapshot.Listeners) == 0 {
			fmt.Fprintf(os.Stderr,
				"warning: no routes found for HTTPRoute %s/%s — check name, namespace, and that the HTTPRoute is attached to this Gateway\n",
				httprouteNamespace, httprouteName)
		}
	}

	// Render and print.
	fmt.Println(renderer.Render(snapshot))
	return nil
}
