package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/genshibangou16/confluence-cli/internal/client"
)

// Page represents a Confluence page
type Page struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Title   string `json:"title"`
	SpaceID string `json:"spaceId"`
	Body    *Body  `json:"body,omitempty"`
	Version *Version `json:"version,omitempty"`
}

// Body represents the page body content
type Body struct {
	Storage *Storage `json:"storage,omitempty"`
}

// Storage represents the storage format content
type Storage struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// Version represents page version info
type Version struct {
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
}

// PageList represents a list of pages response
type PageList struct {
	Results []Page `json:"results"`
	Links   *Links `json:"_links,omitempty"`
}

// Links represents pagination links
type Links struct {
	Next string `json:"next,omitempty"`
}

// CreatePageRequest represents the request body for creating a page
type CreatePageRequest struct {
	SpaceID  string `json:"spaceId"`
	Status   string `json:"status"`
	Title    string `json:"title"`
	ParentID string `json:"parentId,omitempty"`
	Body     Body   `json:"body"`
}

// UpdatePageRequest represents the request body for updating a page
type UpdatePageRequest struct {
	ID      string  `json:"id"`
	Status  string  `json:"status"`
	Title   string  `json:"title"`
	Body    Body    `json:"body"`
	Version Version `json:"version"`
}

// PageService handles page-related API operations
type PageService struct {
	client *client.Client
}

// NewPageService creates a new PageService
func NewPageService() (*PageService, error) {
	c, err := client.NewClient()
	if err != nil {
		return nil, err
	}
	return &PageService{client: c}, nil
}

// ListPages retrieves a list of pages
func (s *PageService) ListPages(spaceID string, limit int) (*PageList, error) {
	params := url.Values{}
	if spaceID != "" {
		params.Set("space-id", spaceID)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	// Include body in response for better output
	params.Set("body-format", "storage")

	path := "/api/v2/pages"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := s.client.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var pageList PageList
	if err := json.NewDecoder(resp.Body).Decode(&pageList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &pageList, nil
}

// GetPage retrieves a single page by ID
func (s *PageService) GetPage(pageID string, includeBody bool) (*Page, error) {
	params := url.Values{}
	if includeBody {
		params.Set("body-format", "storage")
	}

	path := "/api/v2/pages/" + pageID
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := s.client.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// CreatePage creates a new page
func (s *PageService) CreatePage(spaceID, title, body, parentID string) (*Page, error) {
	reqBody := CreatePageRequest{
		SpaceID: spaceID,
		Status:  "current",
		Title:   title,
		Body: Body{
			Storage: &Storage{
				Value:          body,
				Representation: "storage",
			},
		},
	}
	if parentID != "" {
		reqBody.ParentID = parentID
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := s.client.Post("/api/v2/pages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// UpdatePage updates an existing page
func (s *PageService) UpdatePage(pageID, title, body string) (*Page, error) {
	// First, get the current page to obtain the version number
	currentPage, err := s.GetPage(pageID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get current page: %w", err)
	}

	if currentPage.Version == nil {
		return nil, fmt.Errorf("page version not found")
	}

	reqBody := UpdatePageRequest{
		ID:     pageID,
		Status: "current",
		Title:  title,
		Body: Body{
			Storage: &Storage{
				Value:          body,
				Representation: "storage",
			},
		},
		Version: Version{
			Number: currentPage.Version.Number + 1,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := s.client.Put("/api/v2/pages/"+pageID, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &page, nil
}

// DeletePage deletes a page by ID
func (s *PageService) DeletePage(pageID string) error {
	resp, err := s.client.Delete("/api/v2/pages/" + pageID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return err
	}

	return nil
}

// SearchPages searches for pages by title
func (s *PageService) SearchPages(title string, spaceID string, limit int) (*PageList, error) {
	params := url.Values{}
	if title != "" {
		params.Set("title", title)
	}
	if spaceID != "" {
		params.Set("space-id", spaceID)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v2/pages"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := s.client.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var pageList PageList
	if err := json.NewDecoder(resp.Body).Decode(&pageList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &pageList, nil
}

// checkResponse checks the HTTP response for errors
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
}
