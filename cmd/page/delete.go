package page

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <page-id>",
	Short: "Delete a Confluence page",
	Long:  `Delete a Confluence page by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
	pageID := args[0]

	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	if err := service.DeletePage(pageID); err != nil {
		return err
	}

	fmt.Printf("Page %s deleted successfully!\n", pageID)
	return nil
}
