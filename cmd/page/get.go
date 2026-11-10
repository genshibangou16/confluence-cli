package page

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	getFormat string
)

var getCmd = &cobra.Command{
	Use:   "get <page-id>",
	Short: "Get a Confluence page",
	Long:  `Retrieve a single Confluence page by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func init() {
	getCmd.Flags().StringVar(&getFormat, "format", "text", "Output format (text, json)")
}

func runGet(cmd *cobra.Command, args []string) error {
	pageID := args[0]

	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	page, err := service.GetPage(pageID, true)
	if err != nil {
		return err
	}

	switch getFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(page)
	default:
		return outputText(page)
	}
}

func outputText(page *api.Page) error {
	fmt.Printf("ID:       %s\n", page.ID)
	fmt.Printf("Title:    %s\n", page.Title)
	fmt.Printf("Status:   %s\n", page.Status)
	fmt.Printf("Space ID: %s\n", page.SpaceID)

	if page.Version != nil {
		fmt.Printf("Version:  %d\n", page.Version.Number)
	}

	if page.Body != nil && page.Body.Storage != nil {
		fmt.Printf("\nContent:\n%s\n", page.Body.Storage.Value)
	}

	return nil
}
