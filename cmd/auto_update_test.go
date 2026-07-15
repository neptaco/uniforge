package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestAutomaticUpdateEligibilityProtectsMachineReadableOutput(t *testing.T) {
	viper.Set("update.check", true)
	t.Cleanup(func() { viper.Set("update.check", nil) })
	withoutEnv(t, "UNIFORGE_NO_UPDATE_CHECK")
	withoutEnv(t, "CI")
	originalStdoutTTY := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdoutIsTerminal = originalStdoutTTY })

	root := &cobra.Command{Use: "uniforge"}
	plain := &cobra.Command{Use: "doctor"}
	root.AddCommand(plain)
	if !automaticUpdateEligible(plain) {
		t.Fatal("plain text command should allow automatic update checks")
	}

	jsonCmd := &cobra.Command{Use: "info"}
	jsonCmd.Flags().String("output", "", "")
	if err := jsonCmd.Flags().Set("output", "json"); err != nil {
		t.Fatal(err)
	}
	root.AddCommand(jsonCmd)
	if automaticUpdateEligible(jsonCmd) {
		t.Fatal("JSON command must suppress automatic update checks")
	}

	tool := &cobra.Command{Use: "tool"}
	toolCall := &cobra.Command{Use: "call"}
	tool.AddCommand(toolCall)
	root.AddCommand(tool)
	if automaticUpdateEligible(toolCall) {
		t.Fatal("tool protocol output must suppress automatic update checks")
	}
}

func TestAutomaticUpdateEligibilityProtectsAutoDetectedTSV(t *testing.T) {
	viper.Set("update.check", true)
	t.Cleanup(func() { viper.Set("update.check", nil) })
	withoutEnv(t, "UNIFORGE_NO_UPDATE_CHECK")
	withoutEnv(t, "CI")
	originalStdoutTTY := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdoutIsTerminal = originalStdoutTTY })

	root := &cobra.Command{Use: "uniforge"}
	project := &cobra.Command{Use: "project"}
	list := &cobra.Command{Use: "list"}
	list.Flags().String("output", "", "")
	project.AddCommand(list)
	root.AddCommand(project)
	if automaticUpdateEligible(list) {
		t.Fatal("non-TTY project list defaults to TSV and must be protected")
	}
}

func TestAutomaticUpdateNotifyModes(t *testing.T) {
	originalStderrTTY := stderrIsTerminal
	stderrIsTerminal = func() bool { return false }
	t.Cleanup(func() { stderrIsTerminal = originalStderrTTY })

	viper.Set("update.notify", "auto")
	if shouldDisplayAutomaticUpdate() {
		t.Fatal("auto mode should not notify on non-TTY stderr")
	}
	viper.Set("update.notify", "always")
	if !shouldDisplayAutomaticUpdate() {
		t.Fatal("always mode should notify on non-TTY stderr")
	}
	viper.Set("update.notify", "never")
	if shouldDisplayAutomaticUpdate() {
		t.Fatal("never mode should not notify")
	}
	t.Cleanup(func() { viper.Set("update.notify", nil) })
}

func withoutEnv(t *testing.T, name string) {
	t.Helper()
	value, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
