# confluence-cli

Atlassian Confluence CLI tool built with Go.

## Installation

### Using `go install` (recommended)

```bash
go install github.com/genshibangou16/confluence-cli@latest
```

The binary will be installed to `$GOPATH/bin` (usually `~/go/bin`).

### Download binary

Download the latest binary from [GitHub Releases](https://github.com/genshibangou16/confluence-cli/releases) and place it in your PATH:

```bash
# macOS (Apple Silicon)
sudo curl -L https://github.com/genshibangou16/confluence-cli/releases/latest/download/confluence-darwin-arm64 -o /usr/local/bin/confluence
sudo chmod +x /usr/local/bin/confluence

# macOS (Intel)
sudo curl -L https://github.com/genshibangou16/confluence-cli/releases/latest/download/confluence-darwin-amd64 -o /usr/local/bin/confluence
sudo chmod +x /usr/local/bin/confluence

# Linux
sudo curl -L https://github.com/genshibangou16/confluence-cli/releases/latest/download/confluence-linux-amd64 -o /usr/local/bin/confluence
sudo chmod +x /usr/local/bin/confluence
```

### Build from source

```bash
git clone https://github.com/genshibangou16/confluence-cli.git
cd confluence-cli
make install  # installs to $GOPATH/bin (usually ~/go/bin)
```

## Configuration

Configuration is loaded with the following precedence (highest first):
1. Environment variables
2. User config file (`~/.config/confluence-cli/config.yaml`)
3. Project-local config file (`.confluence-cli.yaml`)

### Quick Setup

```bash
# Interactive setup for personal credentials
confluence config init
```

### Team Setup

For teams, create a shared project-local config with the base URL:

```bash
# In your project directory
confluence config init-local
```

This creates `.confluence-cli.yaml` with just the base URL, which can be committed to version control. Each team member then runs `confluence config init` to add their personal credentials.

### Manual Configuration

**User config** (`~/.config/confluence-cli/config.yaml`):
```yaml
base_url: https://your-domain.atlassian.net/wiki
email: your-email@example.com
api_token: your-api-token
```

**Project-local config** (`.confluence-cli.yaml`):
```yaml
base_url: https://your-domain.atlassian.net/wiki
```

**Environment variables**:
```bash
export CONFLUENCE_BASE_URL="https://your-domain.atlassian.net/wiki"
export CONFLUENCE_EMAIL="your-email@example.com"
export CONFLUENCE_API_TOKEN="your-api-token"
```

### API Token

Generate your API token at: https://id.atlassian.com/manage-profile/security/api-tokens

Both classic API tokens and scoped API tokens are supported.
Simply set `base_url` to `https://your-domain.atlassian.net/wiki` and either token type will work.

When using scoped API tokens, the following scopes are required:
- `read:page:confluence`
- `search:confluence`

## Usage

```bash
# Page commands
confluence page list [--space-id <id>] [--limit <n>] [--format table|json]
confluence page get <page-id> [--format text|json]
confluence page create --space-id <id> --title <title> [--body <content>] [--parent-id <id>]
confluence page update <page-id> --title <title> [--body <content>]
confluence page delete <page-id>

# Search (full-text search using CQL)
confluence search <query> [--space-key <key>] [--limit <n>]   # Content search
confluence search <query> --title                              # Title only search
confluence search --cql "type=page AND text ~ 'keyword'"       # CQL query

# Label commands
confluence label list [--prefix <prefix>] [--limit <n>] [--format table|json]
confluence label pages <label-id> [--space-id <id>] [--limit <n>] [--format table|json]

# Config commands
confluence config init         # Setup user credentials
confluence config init-local   # Setup project-local config
confluence config show         # Show current config
confluence config path         # Show config file paths
```

## API Reference

- Uses Confluence REST API v2 (pages, labels)
- Uses Confluence REST API v1 (CQL search)
- [API v2 Documentation](https://developer.atlassian.com/cloud/confluence/rest/v2/intro#about)
- [CQL Documentation](https://developer.atlassian.com/cloud/confluence/advanced-searching-using-cql/)
