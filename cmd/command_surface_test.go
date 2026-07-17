package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCanonicalProjectCommandsAreTopLevel(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "clean", cmd: cleanCmd},
		{name: "doctor", cmd: doctorCmd},
		{name: "open", cmd: openCmd},
		{name: "compile", cmd: compileCmd},
		{name: "test", cmd: testCmd},
		{name: "run", cmd: runCmd},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.cmd.Parent() != rootCmd {
				t.Fatalf("parent = %v, want root command", test.cmd.Parent())
			}
			if test.cmd.Hidden {
				t.Fatal("canonical command must be visible")
			}
			if test.cmd.Deprecated != "" {
				t.Fatalf("canonical command is deprecated: %s", test.cmd.Deprecated)
			}
		})
	}
}

func TestCleanAndDoctorAcceptProjectDirectly(t *testing.T) {
	if cleanCmd.Use != "clean [project]" {
		t.Fatalf("clean use = %q", cleanCmd.Use)
	}
	if cleanCmd.Flags().Lookup("target") == nil || cleanCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("clean command is missing target or dry-run flag")
	}

	if doctorCmd.Use != "doctor [project]" {
		t.Fatalf("doctor use = %q", doctorCmd.Use)
	}
	if doctorCmd.Flags().Lookup("fix") == nil {
		t.Fatal("doctor command is missing fix flag")
	}
}

func TestLegacyCommandFormsRemainHiddenAndDeprecated(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
	}{
		{name: "clean unity", cmd: cleanUnityCmd},
		{name: "doctor unity", cmd: doctorUnityCmd},
		{name: "project open", cmd: projectOpenCmd},
		{name: "batch", cmd: batchCmd},
		{name: "batch compile", cmd: batchCompileCmd},
		{name: "batch test", cmd: batchTestCmd},
		{name: "batch run", cmd: batchRunCmd},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.cmd.Hidden {
				t.Fatal("legacy command must be hidden")
			}
			if test.cmd.Deprecated == "" {
				t.Fatal("legacy command must provide a deprecation message")
			}
		})
	}
}

func TestRootHelpShowsOnlyCanonicalProjectCommands(t *testing.T) {
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil) })

	if err := rootCmd.Help(); err != nil {
		t.Fatalf("render root help: %v", err)
	}
	help := output.String()
	for _, command := range []string{"clean", "doctor", "compile", "test", "run"} {
		if !strings.Contains(help, "  "+command+" ") {
			t.Errorf("root help does not contain canonical command %q", command)
		}
	}
	if strings.Contains(help, "  batch ") {
		t.Error("root help contains deprecated batch command group")
	}
}

func TestManagementParentsShowHelpWithoutRunningAnAction(t *testing.T) {
	if metaCmd.Flags().Lookup("fix") != nil || metaCmd.Flags().Lookup("force") != nil {
		t.Fatal("meta parent must require the explicit check subcommand")
	}

	for _, cmd := range []*cobra.Command{cacheCmd, metaCmd} {
		cmd.SetOut(io.Discard)
		t.Cleanup(func() { cmd.SetOut(nil) })
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("%s parent returned an error: %v", cmd.Name(), err)
		}
	}
}
