package cli

import (
	"bytes"
	"testing"
)

func TestCommandsRegistered(t *testing.T) {
	commands := rootCmd.Commands()
	names := make(map[string]bool)
	for _, cmd := range commands {
		names[cmd.Name()] = true
	}

	expected := []string{"init", "generate", "version"}
	for _, exp := range expected {
		if !names[exp] {
			t.Errorf("expected command '%s' to be registered on rootCmd", exp)
		}
	}
}

func TestVersionCommandOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
}

