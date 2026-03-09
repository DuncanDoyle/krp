package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DuncanDoyle/kfp/internal/parser"
	"github.com/DuncanDoyle/kfp/internal/renderer"
)

// TestParseAllScenarios walks testdata/scenarios/ and parses every config_dump.json found.
// This ensures the parser handles all real config dump variants without errors.
func TestParseAllScenarios(t *testing.T) {
	scenariosDir := "../../testdata/scenarios"

	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		t.Fatalf("reading scenarios dir: %v", err)
	}

	parsed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dumpPath := filepath.Join(scenariosDir, entry.Name(), "envoy", "config_dump.json")
		data, err := os.ReadFile(dumpPath)
		if err != nil {
			// Skip scenarios without config dump
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			snapshot, err := parser.Parse(data)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if len(snapshot.Listeners) == 0 {
				t.Error("expected at least one listener")
			}

			// Verify rendering doesn't panic
			output := renderer.Render(snapshot)
			if output == "" {
				t.Error("expected non-empty render output")
			}

			t.Logf("Scenario %s: %d listeners, render length %d", entry.Name(), len(snapshot.Listeners), len(output))
		})

		parsed++
	}

	if parsed == 0 {
		t.Fatal("no config_dump.json files found in testdata/scenarios/")
	}

	t.Logf("Successfully parsed %d scenarios", parsed)
}
