package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	editorVersionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("43"))
	editorPathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	editorListFormat   string
)

var editorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed Unity Editor versions",
	Long:  `List all installed Unity Editor versions managed by Unity Hub.`,
	RunE:  runList,
}

func init() {
	editorCmd.AddCommand(editorListCmd)

	addOutputFlag(editorListCmd, &editorListFormat, "Output format: text, json, tsv")
}

func runList(cmd *cobra.Command, args []string) error {
	ui.Debug("Listing installed Unity Editor versions")

	editors, err := ui.WithSpinner("Fetching installed editors...", func() ([]hub.EditorInfo, error) {
		hubClient := hub.NewClient()
		return hubClient.ListInstalledEditors()
	})
	if err != nil {
		return fmt.Errorf("failed to list editors: %w", err)
	}

	if len(editors) == 0 {
		if editorListFormat == "json" {
			fmt.Println("[]")
			return nil
		}
		ui.Info("No Unity Editor installations found")
		return nil
	}

	sort.Slice(editors, func(i, j int) bool {
		return compareVersionStrings(editors[i].Version, editors[j].Version) > 0
	})

	format := editorListFormat
	if format == "" {
		if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			format = "text"
		} else {
			format = "tsv"
		}
	}

	switch format {
	case "json":
		return printEditorListJSON(editors)
	case "tsv":
		return printEditorListTSV(editors)
	case "text", "table":
	default:
		return fmt.Errorf("unknown format: %s", format)
	}

	rows := make([][]string, 0, len(editors))
	for _, editor := range editors {
		rows = append(rows, []string{editor.Version, editor.Architecture, editor.Path})
	}

	t := table.New().
		Headers("VERSION", "ARCH", "PATH").
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			switch col {
			case 0:
				return editorVersionStyle
			case 2:
				return editorPathStyle
			}
			return lipgloss.NewStyle()
		})

	fmt.Println(t)
	return nil
}

func printEditorListJSON(editors []hub.EditorInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(editors)
}

func printEditorListTSV(editors []hub.EditorInfo) error {
	for _, editor := range editors {
		fmt.Printf("%s\t%s\t%s\n", editor.Version, editor.Architecture, editor.Path)
	}
	return nil
}
