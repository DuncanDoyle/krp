package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DuncanDoyle/kfp/internal/envoy"
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

	dump.Flags().String("file", "", "Path to an Envoy config_dump JSON file")
	dump.Flags().String("gateway", "", "Gateway name (fetches config via port-forward to gateway-proxy pod)")
	dump.Flags().StringP("namespace", "n", "default", "Namespace of the Gateway (used with --gateway)")
	dump.Flags().String("context", "", "Kubeconfig context (used with --gateway, default: current context)")

	root.AddCommand(dump)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDump(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	gateway, _ := cmd.Flags().GetString("gateway")
	namespace, _ := cmd.Flags().GetString("namespace")
	kubeContext, _ := cmd.Flags().GetString("context")

	if file == "" && gateway == "" {
		return fmt.Errorf("specify either --file <path> or --gateway <name>")
	}
	if file != "" && gateway != "" {
		return fmt.Errorf("--file and --gateway are mutually exclusive")
	}

	// Get the raw config dump bytes
	var data []byte
	var err error

	if file != "" {
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
	} else {
		// Port-forward to the gateway-proxy pod and fetch config dump
		fmt.Fprintln(os.Stderr, "Connecting to Envoy admin API...")
		ctx := context.Background()
		pf, err := envoy.PortForwardToGateway(ctx, gateway, namespace, kubeContext)
		if err != nil {
			return fmt.Errorf("cannot reach Envoy admin API: %w", err)
		}
		defer pf.Stop()

		data, err = envoy.FetchConfigDump(pf.LocalAddr)
		if err != nil {
			return fmt.Errorf("fetching config dump: %w", err)
		}
	}

	// Parse the config dump into an EnvoySnapshot
	result, err := parser.Parse(data)
	if err != nil {
		return err
	}

	// Print any non-fatal parse warnings to stderr so users know if sections were skipped
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	// Render and print
	fmt.Println(renderer.Render(result.Snapshot))
	return nil
}
