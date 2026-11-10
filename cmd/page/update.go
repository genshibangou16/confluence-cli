package page

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	updateTitle string
	updateBody  string
)

var updateCmd = &cobra.Command{
	Use:   "update <page-id>",
	Short: "Update an existing Confluence page",
	Long:  `Update the title and/or content of an existing Confluence page.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "New page title (required)")
	updateCmd.Flags().StringVar(&updateBody, "body", "", "New page body content (storage format)")

	updateCmd.MarkFlagRequired("title")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	pageID := args[0]

	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	page, err := service.UpdatePage(pageID, updateTitle, updateBody)
	if err != nil {
		return err
	}

	fmt.Println("Page updated successfully!")
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(page)
}
