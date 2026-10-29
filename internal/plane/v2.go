package plane

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type v2Adapter struct{}

func (v2Adapter) issueBody(input IssueInput) (json.RawMessage, error) {
	return input.payload([]string{"state_id", "assignee_ids", "label_ids", "parent_id", "estimate_point_id"})
}
func (v2Adapter) page(raw json.RawMessage, query url.Values) (Page, error) {
	var envelope struct {
		Data       json.RawMessage `json:"data"`
		NextCursor *string         `json:"next_cursor"`
		HasMore    bool            `json:"has_more"`
		Next       *int            `json:"next"`
		Pagination struct {
			Style string `json:"style"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Page{}, fmt.Errorf("invalid v2 list response")
	}
	page := Page{}
	var err error
	page.Items, err = decodeItems(envelope.Data)
	if err != nil {
		return page, err
	}
	switch envelope.Pagination.Style {
	case "cursor":
		if envelope.HasMore {
			page.Next, err = cursorQuery(query, envelope.NextCursor)
		}
	case "offset":
		if envelope.Next != nil {
			previous, _ := strconv.Atoi(query.Get("offset"))
			if *envelope.Next <= previous || *envelope.Next > 10000 {
				return page, fmt.Errorf("invalid pagination: offset did not advance or exceeds 10000; use cursor pagination")
			}
			page.Next = cloneQuery(query)
			page.Next.Set("offset", strconv.Itoa(*envelope.Next))
		}
	default:
		return page, fmt.Errorf("invalid v2 pagination style")
	}
	return page, err
}
