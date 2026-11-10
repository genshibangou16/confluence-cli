package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/genshibangou16/confluence-cli/internal/client"
)

// SearchResult represents a search result from CQL query
type SearchResult struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	Status  string       `json:"status"`
	Title   string       `json:"title"`
	Space   *SpaceInfo   `json:"space,omitempty"`
	Excerpt string       `json:"excerpt,omitempty"`
	URL     string       `json:"url,omitempty"`
	Links   *ContentLinks `json:"_links,omitempty"`
}

// SpaceInfo represents space information in search results
type SpaceInfo struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ContentLinks represents links in content
type ContentLinks struct {
	WebUI string `json:"webui,omitempty"`
}

// SearchResponse represents the response from CQL search
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Start   int            `json:"start"`
	Limit   int            `json:"limit"`
	Size    int            `json:"size"`
	Links   *Links         `json:"_links,omitempty"`
}

// SearchService handles search-related API operations
type SearchService struct {
	client *client.Client
}

// NewSearchService creates a new SearchService
func NewSearchService() (*SearchService, error) {
	c, err := client.NewClient()
	if err != nil {
		return nil, err
	}
	return &SearchService{client: c}, nil
}

// SearchByCQL performs a search using CQL (Confluence Query Language)
// Uses v1 API: /rest/api/content/search
func (s *SearchService) SearchByCQL(cql string, limit int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("cql", cql)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	// Include excerpt for content preview
	params.Set("excerpt", "highlight")

	path := "/rest/api/content/search?" + params.Encode()

	resp, err := s.client.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &searchResp, nil
}

// SearchContent searches for content containing the specified text
// Convenience method that builds CQL for text search
func (s *SearchService) SearchContent(text string, spaceKey string, contentType string, limit int) (*SearchResponse, error) {
	// Build CQL query
	cql := fmt.Sprintf("text ~ \"%s\"", text)

	if spaceKey != "" {
		cql += fmt.Sprintf(" AND space = \"%s\"", spaceKey)
	}

	if contentType != "" {
		cql += fmt.Sprintf(" AND type = \"%s\"", contentType)
	}

	return s.SearchByCQL(cql, limit)
}
