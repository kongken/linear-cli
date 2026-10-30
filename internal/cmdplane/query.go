package cmdplane

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func (s *settings) queryCommand(mineCommand bool) *cobra.Command {
	f := new(listFlags)
	var state, group, priority, assignee, created, updated, search, sort string
	var mine, unassigned, allProjects bool
	use, short := "query", "Query work items using supported server-side filters"
	if mineCommand {
		use, short = "mine", "List work items assigned to the current user"
		mine = true
	}
	cmd := &cobra.Command{Use: use, Short: short, Args: exact(0), RunE: s.run("query Plane work items", func(cmd *cobra.Command, _ []string) error {
		count := 0
		for _, v := range []bool{assignee != "", mine, unassigned} {
			if v {
				count++
			}
		}
		if count > 1 {
			return invalid("--assignee, --mine and --unassigned are mutually exclusive")
		}
		if allProjects && cmd.Flags().Changed("project") {
			return invalid("--all-projects and --project are mutually exclusive")
		}
		if priority != "" {
			switch priority {
			case "none", "urgent", "high", "medium", "low":
			default:
				return invalid("invalid priority")
			}
		}
		if group != "" {
			switch group {
			case "backlog", "unstarted", "started", "completed", "cancelled", "triage":
			default:
				return invalid("invalid state group")
			}
		}
		order := strings.TrimPrefix(sort, "-")
		if sort != "" {
			switch order {
			case "created_at", "updated_at", "sequence_id", "id", "sort_order", "priority", "state_group":
			default:
				return invalid("unsupported sort field")
			}
		}
		if f.Pagination == "cursor" || f.Cursor != "" {
			if order == "sort_order" || order == "priority" || order == "state_group" {
				return invalid("this sort requires offset pagination")
			}
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		q, err := f.query(cmd, c)
		if err != nil {
			return err
		}
		hasFilter := state != "" || group != "" || priority != "" || assignee != "" || mine || unassigned || created != "" || updated != "" || search != "" || allProjects
		if c.Context.APIVersion == "v1" && hasFilter {
			return invalid("these query filters are not verified for the v1 API; use issue list or an explicit plane api request")
		}
		if c.Context.APIVersion == "v1" && (order == "priority" || order == "state_group") {
			return invalid("semantic sorting is not supported by this v1 adapter")
		}
		project := ""
		if !allProjects {
			project, err = resolveProject(cmd.Context(), c, "", true)
			if err != nil {
				return err
			}
		}
		if state != "" {
			if project == "" {
				id, e := uuid.Parse(state)
				if e != nil {
					return invalid("workspace queries require a state UUID")
				}
				q.Set("state_id", id.String())
			} else {
				id, e := resolveRelation(cmd.Context(), c, "states", project, state)
				if e != nil {
					return e
				}
				q.Set("state_id", id)
			}
		}
		if group != "" {
			q.Set("state_group", group)
		}
		if priority != "" {
			q.Set("priority", priority)
		}
		if mine {
			assignee = "me"
		}
		if assignee != "" {
			id, e := resolveRelation(cmd.Context(), c, "members", project, assignee)
			if e != nil {
				return e
			}
			q.Set("assignee_id", id)
		}
		if unassigned {
			q.Set("assignee_id__isnull", "true")
		}
		for key, value := range map[string]string{"created_at__gte": created, "updated_at__gte": updated} {
			if value == "" {
				continue
			}
			when, e := time.Parse(time.RFC3339, value)
			if e != nil {
				when, e = time.Parse("2006-01-02", value)
			}
			if e != nil {
				return invalid("date filters require YYYY-MM-DD or RFC3339")
			}
			q.Set(key, when.Format(time.RFC3339))
		}
		if search != "" {
			q.Set("search", search)
		}
		if sort != "" {
			q.Set("order_by", sort)
		}
		path, err := c.ResourcePath("work-items", project)
		if err != nil {
			return err
		}
		return f.output(cmd, c, path, q)
	})}
	f.bind(cmd)
	cmd.Flags().StringVar(&state, "state", "", "Project state UUID or unique name")
	cmd.Flags().StringVar(&group, "state-group", "", "Workflow state group")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority filter")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee UUID, unique email or me")
	if !mineCommand {
		cmd.Flags().BoolVar(&mine, "mine", false, "Only work items assigned to the current user")
	}
	cmd.Flags().BoolVar(&unassigned, "unassigned", false, "Only work items with no assignees")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query across the workspace (v2 only)")
	cmd.Flags().StringVar(&created, "created-after", "", "Created on/after a date or timestamp")
	cmd.Flags().StringVar(&updated, "updated-after", "", "Updated on/after a date or timestamp")
	cmd.Flags().StringVar(&search, "search", "", "Search work-item names (v2)")
	cmd.Flags().StringVar(&sort, "sort", "", fmt.Sprintf("Order by %s (prefix - for descending)", "created_at, updated_at, sequence_id, id, sort_order, priority or state_group"))
	return cmd
}
