package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/genshibangou16/confluence-cli/internal/api"
)

var (
	searchSpaceKey  string
	searchLimit     int
	searchFormat    string
	searchCQL       string
	searchTitleOnly bool
	searchHelpCQL   bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for content",
	Long: `Search for Confluence content using full-text search or CQL.

By default, searches content (body text). Use --title to search titles only.
Use --cql for advanced CQL queries.

Examples:
  confluence search "error handling"              # Full-text search
  confluence search "API" --space-key DEV         # Search in specific space
  confluence search "setup" --title               # Search titles only
  confluence search --cql "type=page AND label=important"  # CQL query`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVar(&searchSpaceKey, "space-key", "", "Filter results by space key")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 25, "Maximum number of results to return")
	searchCmd.Flags().StringVar(&searchFormat, "format", "table", "Output format (table, json)")
	searchCmd.Flags().StringVar(&searchCQL, "cql", "", "Search using CQL (Confluence Query Language)")
	searchCmd.Flags().BoolVar(&searchTitleOnly, "title", false, "Search titles only (faster)")
	searchCmd.Flags().BoolVar(&searchHelpCQL, "help-cql", false, "Show CQL syntax help and examples")
}

func runSearch(cmd *cobra.Command, args []string) error {
	// Show CQL help
	if searchHelpCQL {
		printCQLHelp()
		return nil
	}

	// CQL mode
	if searchCQL != "" {
		return runCQLSearch(searchCQL)
	}

	// Need a query if not using CQL
	if len(args) == 0 {
		return fmt.Errorf("query is required (or use --cql for CQL search)")
	}
	query := args[0]

	// Title-only search (v2 API)
	if searchTitleOnly {
		return runTitleSearch(query)
	}

	// Full-text search (v1 API with CQL)
	return runContentSearch(query)
}

func runTitleSearch(query string) error {
	service, err := api.NewPageService()
	if err != nil {
		return err
	}

	// Convert space-key to space-id if needed (title search uses v2 API)
	pages, err := service.SearchPages(query, "", searchLimit)
	if err != nil {
		return err
	}

	if len(pages.Results) == 0 {
		fmt.Println("No pages found.")
		return nil
	}

	if searchFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pages)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tSPACE ID")
	fmt.Fprintln(w, "--\t-----\t------\t--------")
	for _, p := range pages.Results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, p.Title, p.Status, p.SpaceID)
	}
	return w.Flush()
}

func runContentSearch(query string) error {
	service, err := api.NewSearchService()
	if err != nil {
		return err
	}

	results, err := service.SearchContent(query, searchSpaceKey, "page", searchLimit)
	if err != nil {
		return err
	}

	return outputSearchResults(results)
}

func runCQLSearch(cql string) error {
	service, err := api.NewSearchService()
	if err != nil {
		return err
	}

	results, err := service.SearchByCQL(cql, searchLimit)
	if err != nil {
		return err
	}

	return outputSearchResults(results)
}

func outputSearchResults(results *api.SearchResponse) error {
	if len(results.Results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	if searchFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSPACE\tEXCERPT")
	fmt.Fprintln(w, "--\t-----\t----\t-----\t-------")
	for _, r := range results.Results {
		spaceKey := ""
		if r.Space != nil {
			spaceKey = r.Space.Key
		}
		// Truncate and clean excerpt
		excerpt := cleanExcerpt(r.Excerpt, 50)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Title, r.Type, spaceKey, excerpt)
	}
	return w.Flush()
}

func cleanExcerpt(excerpt string, maxLen int) string {
	// Remove HTML tags
	excerpt = strings.ReplaceAll(excerpt, "<b>", "")
	excerpt = strings.ReplaceAll(excerpt, "</b>", "")
	excerpt = strings.ReplaceAll(excerpt, "@@@hl@@@", "")
	excerpt = strings.ReplaceAll(excerpt, "@@@endhl@@@", "")
	excerpt = strings.ReplaceAll(excerpt, "\n", " ")
	excerpt = strings.TrimSpace(excerpt)

	if len(excerpt) > maxLen {
		excerpt = excerpt[:maxLen] + "..."
	}
	return excerpt
}

func printCQLHelp() {
	help := `
CQL (Confluence Query Language) Help
=====================================

SYNTAX
  field operator value [AND|OR field operator value]

FIELDS
  text          Body text search
  title         Title search
  space         Space key (e.g., DEV, MARKETING)
  type          Content type (page, blogpost, comment, attachment)
  label         Label name
  creator       Creator's account ID
  created       Creation date
  lastmodified  Last modified date

OPERATORS
  =             Exact match
  ~             Contains (text search)
  !=            Not equal
  >  <          Greater/less than (for dates)
  >=  <=        Greater/less than or equal (for dates)

EXAMPLES

  # Search pages containing "API" in body
  confluence search --cql "text ~ 'API'"

  # Search pages with "design" in title
  confluence search --cql "title ~ 'design'"

  # Search within a specific space
  confluence search --cql "space = DEV AND text ~ 'error'"

  # Search pages with a label
  confluence search --cql "label = 'important'"

  # Search pages with multiple labels
  confluence search --cql "label = 'api' AND label = 'v2'"

  # Recently updated pages (last 7 days)
  confluence search --cql "lastmodified >= now('-7d')"

  # Pages created in the last 30 days
  confluence search --cql "created >= now('-30d') AND type = page"

  # Pages only (exclude blog posts)
  confluence search --cql "type = page AND text ~ 'manual'"

  # Complex query
  confluence search --cql "space = DEV AND type = page AND label = 'release' AND text ~ '2024'"

DATE FUNCTIONS
  now()         Current time
  now('-1d')    1 day ago
  now('-7d')    7 days ago
  now('-30d')   30 days ago
  now('-1w')    1 week ago
  now('-1M')    1 month ago
  now('-1y')    1 year ago

NOTES
  - Wrap values containing spaces in single quotes
  - AND/OR must be uppercase
  - Use parentheses () for grouping

More info: https://developer.atlassian.com/cloud/confluence/advanced-searching-using-cql/
`
	fmt.Print(help)
}
