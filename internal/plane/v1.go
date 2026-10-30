package plane

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type v1Adapter struct{}

func (v1Adapter) issueBody(input IssueInput) (json.RawMessage, error) {
	return input.payload([]string{"state", "assignees", "labels", "parent", "estimate_point"})
}
func (v1Adapter) page(raw json.RawMessage, query url.Values) (Page, error) {
	var envelope struct {
		Results         json.RawMessage `json:"results"`
		NextCursor      *string         `json:"next_cursor"`
		NextPageResults bool            `json:"next_page_results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Page{}, fmt.Errorf("invalid v1 list response")
	}
	page := Page{}
	var err error
	page.Items, err = decodeItems(envelope.Results)
	if err != nil {
		return page, err
	}
	if envelope.NextPageResults {
		page.Next, err = cursorQuery(query, envelope.NextCursor)
	}
	return page, err
}
