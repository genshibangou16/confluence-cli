package page

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	createSpaceID  string
	createTitle    string
	createBody     string
	createParentID string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Confluence page",
	Long:  `Create a new page in Confluence with the specified title and content.`,
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVar(&createSpaceID, "space-id", "", "Space ID where the page will be created (required)")
	createCmd.Flags().StringVar(&createTitle, "title", "", "Page title (required)")
	createCmd.Flags().StringVar(&createBody, "body", "", "Page body content (storage format)")
	createCmd.Flags().StringVar(&createParentID, "parent-id", "", "Parent page ID (optional)")

	createCmd.MarkFlagRequired("space-id")
	createCmd.MarkFlagRequired("title")
}

func runCreate(cmd *cobra.Command, args []string) error {
	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	page, err := service.CreatePage(createSpaceID, createTitle, createBody, createParentID)
	if err != nil {
		return err
	}

	fmt.Println("Page created successfully!")
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(page)
}
