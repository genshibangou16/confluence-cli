package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/genshibangou16/confluence-cli/internal/client"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `Commands for managing Confluence CLI configuration.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize user configuration file",
	Long: `Create a new user configuration file with your Confluence credentials.
The file is saved to ~/.config/confluence-cli/config.yaml`,
	RunE: runConfigInit,
}

var configInitLocalCmd = &cobra.Command{
	Use:   "init-local",
	Short: "Initialize project-local configuration file",
	Long: `Create a project-local configuration file (.confluence-cli.yaml).
This file can be committed to version control to share base_url with your team.
Each team member still needs their own credentials in their user config.`,
	RunE: runConfigInitLocal,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration and where it's loaded from (API token is masked).`,
	RunE:  runConfigShow,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file paths",
	RunE:  runConfigPath,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configInitLocalCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Confluence CLI - User Configuration")
	fmt.Println("------------------------------------")
	fmt.Println()

	// Check if config already exists
	configPath := client.ConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists at: %s\n", configPath)
		fmt.Print("Overwrite? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Check if local config exists to suggest default base_url
	var defaultBaseURL string
	if localPath := client.FindLocalConfig(); localPath != "" {
		if data, err := os.ReadFile(localPath); err == nil {
			var localConfig client.Config
			if yaml.Unmarshal(data, &localConfig) == nil && localConfig.BaseURL != "" {
				defaultBaseURL = localConfig.BaseURL
			}
		}
	}

	var baseURL string
	if defaultBaseURL != "" {
		fmt.Printf("Base URL [%s]: ", defaultBaseURL)
		baseURL, _ = reader.ReadString('\n')
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
	} else {
		fmt.Print("Base URL (e.g., https://your-domain.atlassian.net/wiki): ")
		baseURL, _ = reader.ReadString('\n')
		baseURL = strings.TrimSpace(baseURL)
	}

	fmt.Print("Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	fmt.Print("API Token: ")
	apiToken, _ := reader.ReadString('\n')
	apiToken = strings.TrimSpace(apiToken)

	config := client.Config{
		BaseURL:  baseURL,
		Email:    email,
		APIToken: apiToken,
	}

	if err := client.SaveConfig(config); err != nil {
		return err
	}

	fmt.Printf("\nConfiguration saved to: %s\n", configPath)
	fmt.Println("\nYou can also override these values with environment variables:")
	fmt.Println("  CONFLUENCE_BASE_URL")
	fmt.Println("  CONFLUENCE_EMAIL")
	fmt.Println("  CONFLUENCE_API_TOKEN")

	return nil
}

func runConfigInitLocal(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Confluence CLI - Project Local Configuration")
	fmt.Println("---------------------------------------------")
	fmt.Println("This creates a .confluence-cli.yaml file in the current directory.")
	fmt.Println("You can commit this file to share base_url with your team.")
	fmt.Println()

	localPath := filepath.Join(".", client.LocalConfigName)
	if _, err := os.Stat(localPath); err == nil {
		fmt.Printf("Local config already exists: %s\n", localPath)
		fmt.Print("Overwrite? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Print("Base URL (e.g., https://your-domain.atlassian.net/wiki): ")
	baseURL, _ := reader.ReadString('\n')
	baseURL = strings.TrimSpace(baseURL)

	// Only save base_url in local config
	content := fmt.Sprintf(`# Confluence CLI - Project Configuration
# This file can be committed to version control.
# Each team member needs their own credentials in ~/.config/confluence-cli/config.yaml

base_url: %s
`, baseURL)

	if err := os.WriteFile(localPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write local config: %w", err)
	}

	fmt.Printf("\nLocal configuration saved to: %s\n", localPath)
	fmt.Println("\nTeam members can run 'confluence config init' to set up their credentials.")
	fmt.Println("The base_url from this file will be used as the default.")

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	fmt.Println("Configuration Sources")
	fmt.Println("---------------------")

	// Show local config if found
	if localPath := client.FindLocalConfig(); localPath != "" {
		fmt.Printf("Project local: %s\n", localPath)
	} else {
		fmt.Println("Project local: (not found)")
	}

	// Show user config
	userPath := client.ConfigPath()
	if _, err := os.Stat(userPath); err == nil {
		fmt.Printf("User config:   %s\n", userPath)
	} else {
		fmt.Printf("User config:   %s (not found)\n", userPath)
	}

	fmt.Println()

	// Show resolved config
	config, err := client.LoadConfig()
	if err != nil {
		fmt.Println("Resolved Configuration")
		fmt.Println("----------------------")
		fmt.Printf("Error: %v\n", err)
		return nil
	}

	fmt.Println("Resolved Configuration")
	fmt.Println("----------------------")
	fmt.Printf("Base URL:  %s\n", config.BaseURL)
	fmt.Printf("Email:     %s\n", config.Email)
	fmt.Printf("API Token: %s\n", maskToken(config.APIToken))

	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	fmt.Println("User config:    ", client.ConfigPath())
	fmt.Println("Local config:   ", client.LocalConfigName)
	if localPath := client.FindLocalConfig(); localPath != "" {
		fmt.Println("Found local at: ", localPath)
	}
	return nil
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "****" + token[len(token)-4:]
}
