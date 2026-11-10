package page

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	listSpaceID string
	listLimit   int
	listFormat  string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Confluence pages",
	Long:  `Retrieve a list of Confluence pages, optionally filtered by space.`,
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVar(&listSpaceID, "space-id", "", "Filter pages by space ID")
	listCmd.Flags().IntVar(&listLimit, "limit", 25, "Maximum number of pages to return")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format (table, json)")
}

func runList(cmd *cobra.Command, args []string) error {
	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	pageList, err := service.ListPages(listSpaceID, listLimit)
	if err != nil {
		return err
	}

	switch listFormat {
	case "json":
		return outputJSON(pageList)
	default:
		return outputTable(pageList)
	}
}

func outputJSON(pageList *api.PageList) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(pageList)
}

func outputTable(pageList *api.PageList) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tSPACE ID")
	fmt.Fprintln(w, "--\t-----\t------\t--------")

	for _, page := range pageList.Results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", page.ID, page.Title, page.Status, page.SpaceID)
	}

	return w.Flush()
}
