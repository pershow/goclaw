package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestClawhubCommandExists(t *testing.T) {
	// Find the clawhub command
	var clawhubCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "clawhub" {
			clawhubCmd = cmd
			break
		}
	}

	if clawhubCmd == nil {
		t.Fatal("clawhub command not found")
	}

	// Check that it has the right description
	if clawhubCmd.Short != "ClawHub skill registry commands" {
		t.Errorf("expected short description 'ClawHub skill registry commands', got '%s'", clawhubCmd.Short)
	}

	// Check that it has some expected subcommands (some may be nested)
	subcommands := clawhubCmd.Commands()
	expectedDirectSubcommands := []string{
		"login", "logout", "whoami",
		"list", "sync",
	}

	found := make(map[string]bool)
	for _, cmd := range subcommands {
		found[cmd.Use] = true
	}

	for _, expected := range expectedDirectSubcommands {
		if !found[expected] {
			t.Errorf("missing direct subcommand: %s", expected)
		}
	}

	// Verify we have at least some commands
	if len(subcommands) < 5 {
		t.Errorf("expected at least 5 subcommands, got %d", len(subcommands))
	}
}

func TestClawhubGlobalFlags(t *testing.T) {
	var clawhubCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "clawhub" {
			clawhubCmd = cmd
			break
		}
	}

	if clawhubCmd == nil {
		t.Fatal("clawhub command not found")
	}

	// Check for global flags
	expectedFlags := []string{"workdir", "dir", "site", "registry", "no-input"}
	flags := clawhubCmd.LocalFlags()

	for _, flagName := range expectedFlags {
		if flags.Lookup(flagName) == nil {
			t.Errorf("missing flag: %s", flagName)
		}
	}
}
