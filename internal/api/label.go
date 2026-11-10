package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/genshibangou16/confluence-cli/internal/client"
)

// Label represents a Confluence label
type Label struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// LabelList represents a list of labels response
type LabelList struct {
	Results []Label `json:"results"`
	Links   *Links  `json:"_links,omitempty"`
}

// LabelService handles label-related API operations
type LabelService struct {
	client *client.Client
}

// NewLabelService creates a new LabelService
func NewLabelService() (*LabelService, error) {
	c, err := client.NewClient()
	if err != nil {
		return nil, err
	}
	return &LabelService{client: c}, nil
}

// ListLabels retrieves all labels
func (s *LabelService) ListLabels(prefix string, limit int) (*LabelList, error) {
	params := url.Values{}
	if prefix != "" {
		params.Set("prefix", prefix)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v2/labels"
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

	var labelList LabelList
	if err := json.NewDecoder(resp.Body).Decode(&labelList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &labelList, nil
}

// GetPageLabels retrieves labels for a specific page
func (s *LabelService) GetPageLabels(pageID string, limit int) (*LabelList, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v2/pages/" + pageID + "/labels"
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

	var labelList LabelList
	if err := json.NewDecoder(resp.Body).Decode(&labelList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &labelList, nil
}

// GetPagesByLabel retrieves pages that have a specific label
func (s *LabelService) GetPagesByLabel(labelID string, spaceID string, limit int) (*PageList, error) {
	params := url.Values{}
	if spaceID != "" {
		params.Set("space-id", spaceID)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v2/labels/" + labelID + "/pages"
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
