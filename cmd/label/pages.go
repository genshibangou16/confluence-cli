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
	pagesSpaceID string
	pagesLimit   int
	pagesFormat  string
)

var pagesCmd = &cobra.Command{
	Use:   "pages <label-id>",
	Short: "Get pages with a specific label",
	Long:  `Retrieve all pages that have a specific label.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPages,
}

func init() {
	pagesCmd.Flags().StringVar(&pagesSpaceID, "space-id", "", "Filter pages by space ID")
	pagesCmd.Flags().IntVar(&pagesLimit, "limit", 25, "Maximum number of pages to return")
	pagesCmd.Flags().StringVar(&pagesFormat, "format", "table", "Output format (table, json)")
}

func runPages(cmd *cobra.Command, args []string) error {
	labelID := args[0]

	service, err := api.NewLabelService()
	if err != nil {
		return err
	}

	pages, err := service.GetPagesByLabel(labelID, pagesSpaceID, pagesLimit)
	if err != nil {
		return err
	}

	if pagesFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pages)
	}

	// Table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tSPACE ID")
	fmt.Fprintln(w, "--\t-----\t------\t--------")
	for _, p := range pages.Results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, p.Title, p.Status, p.SpaceID)
	}
	return w.Flush()
}
