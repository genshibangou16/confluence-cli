package label

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	listPrefix string
	listLimit  int
	listFormat string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all labels",
	Long:  `Retrieve a list of all labels in Confluence.`,
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVar(&listPrefix, "prefix", "", "Filter labels by prefix (my, team, global, system)")
	listCmd.Flags().IntVar(&listLimit, "limit", 25, "Maximum number of labels to return")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format (table, json)")
}

func runList(cmd *cobra.Command, args []string) error {
	service, err := api.NewLabelService()
	if err != nil {
		return err
	}

	labels, err := service.ListLabels(listPrefix, listLimit)
	if err != nil {
		return err
	}

	if listFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(labels)
	}

	// Table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPREFIX")
	fmt.Fprintln(w, "--\t----\t------")
	for _, l := range labels.Results {
		fmt.Fprintf(w, "%s\t%s\t%s\n", l.ID, l.Name, l.Prefix)
	}
	return w.Flush()
}
