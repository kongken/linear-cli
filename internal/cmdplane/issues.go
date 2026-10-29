package cmdplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kongken/linear-cli/internal/plane"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
)

type directoryEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Email      string `json:"email"`
	MemberID   string `json:"member_id"`
	Member     struct {
		Email string `json:"email"`
	} `json:"member"`
}

func resolveProject(ctx context.Context, c *plane.Client, ref string, required bool) (string, error) {
	if ref == "" {
		ref = c.Context.ProjectID
	}
	if ref == "" {
		if required {
			return "", invalid("--project or PLANE_PROJECT_ID is required")
		}
		return "", nil
	}
	if id, err := uuid.Parse(ref); err == nil {
		return id.String(), nil
	}
	rows, err := c.Directory(ctx, "projects", "")
	if err != nil {
		return "", err
	}
	return findEntry(rows, ref, false)
}
func findEntry(rows []json.RawMessage, ref string, member bool) (string, error) {
	if ref == "" {
		return "", invalid("resource reference cannot be empty; use an explicit clear flag")
	}
	found := ""
	for _, raw := range rows {
		var row directoryEntry
		if err := json.Unmarshal(raw, &row); err != nil {
			return "", fmt.Errorf("invalid directory response")
		}
		match := strings.EqualFold(row.Name, ref) || strings.EqualFold(row.Identifier, ref)
		if member {
			match = strings.EqualFold(row.Email, ref) || strings.EqualFold(row.Member.Email, ref)
		}
		if match {
			id := row.ID
			if member && row.MemberID != "" {
				id = row.MemberID
			}
			if id == "" {
				return "", fmt.Errorf("directory entry is missing its user/resource ID")
			}
			if found != "" && found != id {
				return "", invalid("resource name is ambiguous; use its UUID")
			}
			found = id
		}
	}
	if found == "" {
		return "", invalid("resource reference was not found; use a UUID or a unique name/email")
	}
	return found, nil
}
func resolveRelation(ctx context.Context, c *plane.Client, resource, project, ref string) (string, error) {
	if ref == "" {
		return "", invalid("resource reference cannot be empty; use an explicit clear flag")
	}
	if id, err := uuid.Parse(ref); err == nil {
		return id.String(), nil
	}
	if resource == "members" && (ref == "me" || ref == "@me" || ref == "self") {
		raw, err := c.CurrentUser(ctx)
		if err != nil {
			return "", err
		}
		var user struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &user); err != nil || user.ID == "" {
			return "", fmt.Errorf("current user response is missing ID")
		}
		return user.ID, nil
	}
	rows, err := c.Directory(ctx, resource, project)
	if err != nil {
		return "", err
	}
	return findEntry(rows, ref, resource == "members")
}

type listFlags struct {
	JSON       bool
	JSONL      bool
	Limit      int
	PageSize   int
	Cursor     string
	Offset     int
	Pagination string
}

func (f *listFlags) bind(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.JSON, "json", false, "Return one native response page as JSON")
	cmd.Flags().BoolVar(&f.JSONL, "jsonl", false, "Export native resource objects across pages, one JSON object per line")
	cmd.Flags().IntVar(&f.Limit, "limit", 50, "Maximum text/JSONL rows (1–10000; incompatible with --json)")
	cmd.Flags().IntVar(&f.PageSize, "page-size", 50, "API page size")
	cmd.Flags().StringVar(&f.Cursor, "cursor", "", "Opaque continuation cursor")
	cmd.Flags().IntVar(&f.Offset, "offset", 0, "v2 continuation offset")
	cmd.Flags().StringVar(&f.Pagination, "pagination", "", "v2 pagination style: offset or cursor")
}
func (f listFlags) query(cmd *cobra.Command, c *plane.Client) (url.Values, error) {
	if f.JSON && f.JSONL {
		return nil, invalid("--json and --jsonl are mutually exclusive")
	}
	if f.JSON && cmd.Flags().Changed("limit") {
		return nil, invalid("--limit cannot be used with --json; use --page-size or --jsonl")
	}
	if f.Limit < 1 || f.Limit > 10000 {
		return nil, invalid("--limit must be between 1 and 10000")
	}
	max := 200
	if c.Context.APIVersion == "v1" {
		max = 100
	}
	if f.PageSize < 1 || f.PageSize > max {
		return nil, invalid(fmt.Sprintf("--page-size must be between 1 and %d for this API", max))
	}
	if f.Cursor != "" && cmd.Flags().Changed("offset") {
		return nil, invalid("--cursor and --offset are mutually exclusive")
	}
	if f.Offset < 0 || f.Offset > 10000 {
		return nil, invalid("--offset must be between 0 and 10000")
	}
	q := url.Values{"per_page": {fmt.Sprint(f.PageSize)}}
	if c.Context.APIVersion == "v1" {
		if cmd.Flags().Changed("offset") || f.Pagination != "" {
			return nil, invalid("v1 only supports cursor pagination")
		}
	} else {
		if f.Pagination != "" && f.Pagination != "offset" && f.Pagination != "cursor" {
			return nil, invalid("--pagination must be offset or cursor")
		}
		if f.Pagination == "offset" && f.Cursor != "" {
			return nil, invalid("an offset page cannot use --cursor")
		}
		if f.Pagination == "cursor" || f.Cursor != "" {
			q.Set("paginate", "cursor")
		} else if cmd.Flags().Changed("offset") {
			q.Set("offset", fmt.Sprint(f.Offset))
		}
	}
	if f.Cursor != "" {
		q.Set("cursor", f.Cursor)
	}
	return q, nil
}
func (f listFlags) output(cmd *cobra.Command, c *plane.Client, path string, q url.Values) error {
	if f.JSON {
		page, err := c.ListPath(cmd.Context(), path, q)
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), page.Raw)
	}
	seen := map[string]bool{}
	count := 0
	for {
		key := q.Encode()
		if seen[key] {
			return fmt.Errorf("pagination repeated a page")
		}
		seen[key] = true
		page, err := c.ListPath(cmd.Context(), path, q)
		if err != nil {
			return err
		}
		rows := page.Items
		if len(rows) > f.Limit-count {
			rows = rows[:f.Limit-count]
		}
		if f.JSONL {
			for _, row := range rows {
				var compact bytes.Buffer
				if err := json.Compact(&compact, row); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), compact.String()); err != nil {
					return err
				}
			}
		} else if err := writeRows(cmd.OutOrStdout(), rows); err != nil {
			return err
		}
		count += len(rows)
		if count >= f.Limit {
			if page.Next != nil || len(rows) < len(page.Items) {
				_, err := fmt.Fprintln(cmd.ErrOrStderr(), "Result limit reached; more resources may be available.")
				return err
			}
			return nil
		}
		if page.Next == nil {
			return nil
		}
		q = page.Next
	}
}
func (s *settings) directoryCommand(name, resource string) *cobra.Command {
	root := &cobra.Command{Use: name, Short: "Manage Plane " + name + " resources"}
	f := new(listFlags)
	list := &cobra.Command{Use: "list", Short: "List " + name + " resources", Args: exact(0), RunE: s.run("list Plane "+name+" resources", func(cmd *cobra.Command, _ []string) error {
		c, err := s.client()
		if err != nil {
			return err
		}
		q, err := f.query(cmd, c)
		if err != nil {
			return err
		}
		project := ""
		if resource != "projects" {
			project, err = resolveProject(cmd.Context(), c, "", resource != "members")
			if err != nil {
				return err
			}
		}
		if resource == "members" && c.Context.APIVersion == "v2" {
			q.Set("expand", "member")
		}
		path, err := c.ResourcePath(resource, project)
		if err != nil {
			return invalid(err.Error())
		}
		return f.output(cmd, c, path, q)
	})}
	f.bind(list)
	root.AddCommand(list)
	if resource != "members" && resource != "estimates" {
		root.AddCommand(&cobra.Command{Use: "view <id>", Short: "Show a " + name + " resource as native JSON", Args: exact(1), RunE: s.run("view Plane "+name, func(cmd *cobra.Command, args []string) error {
			c, err := s.client()
			if err != nil {
				return err
			}
			project := ""
			id := args[0]
			if resource == "projects" {
				id, err = resolveProject(cmd.Context(), c, id, true)
			} else {
				project, err = resolveProject(cmd.Context(), c, "", true)
			}
			if err != nil {
				return err
			}
			raw, err := c.GetResource(cmd.Context(), resource, project, id)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), raw)
		})})
	}
	if resource != "members" && resource != "estimates" {
		s.resourceAdmin(root, resource)
	}
	return root
}
func (s *settings) issueCommand() *cobra.Command {
	root := &cobra.Command{Use: "issue", Short: "Manage Plane work items"}
	f := new(listFlags)
	list := &cobra.Command{Use: "list", Short: "List work items in a project", Args: exact(0), RunE: s.run("list Plane work items", func(cmd *cobra.Command, _ []string) error {
		c, err := s.client()
		if err != nil {
			return err
		}
		q, err := f.query(cmd, c)
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", true)
		if err != nil {
			return err
		}
		path, err := c.ResourcePath("work-items", project)
		if err != nil {
			return err
		}
		return f.output(cmd, c, path, q)
	})}
	f.bind(list)
	root.AddCommand(list)
	var jsonOut bool
	view := &cobra.Command{Use: "view <issue>", Short: "Show a work item by UUID, identifier or project-scoped number", Args: exact(1), RunE: s.run("view Plane work item", func(cmd *cobra.Command, args []string) error {
		c, err := s.client()
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		raw, err := c.GetIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		// Native JSON also remains available for rich-text details in text mode.
		if jsonOut {
			return writeJSON(cmd.OutOrStdout(), raw)
		}
		var item struct {
			Identifier  string   `json:"identifier"`
			Name        string   `json:"name"`
			HTML        string   `json:"description_html"`
			Priority    string   `json:"priority"`
			Assignees   []string `json:"assignee_ids"`
			V1Assignees []string `json:"assignees"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		people := item.Assignees
		if c.Context.APIVersion == "v1" {
			people = item.V1Assignees
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nPriority: %s\nAssignees: %s\n\n%s\n", item.Identifier, item.Name, item.Priority, strings.Join(people, ", "), plainHTML(item.HTML))
		return err
	})}
	view.Flags().BoolVar(&jsonOut, "json", false, "Return native work-item JSON")
	root.AddCommand(view, s.writeIssueCommand(false), s.writeIssueCommand(true), s.commentCommand(), s.queryCommand(false), s.queryCommand(true))
	s.issueActions(root)
	s.summaryCommands(root)
	root.AddCommand(s.attachmentCommand(), s.startCommand())
	return root
}

type issueFlags struct {
	Title, Description, DescriptionFile, Format, Priority, State, Parent, DueDate, Estimate string
	Assignees, Labels                                                                       []string
	ClearAssignees, ClearLabels, ClearParent, ClearDueDate, ClearEstimate                   bool
	JSON                                                                                    bool
}

func (s *settings) writeIssueCommand(update bool) *cobra.Command {
	f := &issueFlags{}
	use, short := "create", "Create a work item"
	n := 0
	if update {
		use, short = "update <issue>", "Update a work item"
		n = 1
	}
	cmd := &cobra.Command{Use: use, Short: short, Args: exact(n), RunE: s.run("save Plane work item", func(cmd *cobra.Command, args []string) error {
		if err := f.validate(cmd, update); err != nil {
			return err
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", !update)
		if err != nil {
			return err
		}
		var ref plane.IssueRef
		if update {
			ref, err = c.ResolveIssue(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			project = ref.ProjectID
		}
		input, err := f.input(cmd, c, project)
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if update {
			raw, err = c.UpdateIssue(cmd.Context(), ref, input)
		} else {
			raw, err = c.CreateIssue(cmd.Context(), project, input)
		}
		if err != nil {
			return err
		}
		if f.JSON {
			return writeJSON(cmd.OutOrStdout(), raw)
		}
		return writeRows(cmd.OutOrStdout(), []json.RawMessage{raw})
	})}
	cmd.Flags().StringVar(&f.Title, "title", "", "Work-item title")
	cmd.Flags().StringVar(&f.Description, "description", "", "Description content (Markdown by default)")
	cmd.Flags().StringVar(&f.DescriptionFile, "description-file", "", "Read description from a UTF-8 file")
	cmd.Flags().StringVar(&f.Format, "description-format", "markdown", "Content format: markdown or html")
	cmd.Flags().StringVar(&f.Priority, "priority", "", "Priority: none, urgent, high, medium or low")
	cmd.Flags().StringVar(&f.State, "state", "", "Project state UUID or unique name")
	cmd.Flags().StringArrayVar(&f.Assignees, "assignee", nil, "Assignee UUID, unique email or me (repeat to set the full set)")
	cmd.Flags().StringArrayVar(&f.Labels, "label", nil, "Label UUID or unique name (repeat to set the full set)")
	cmd.Flags().StringVar(&f.Parent, "parent", "", "Parent work-item UUID or identifier")
	cmd.Flags().StringVar(&f.DueDate, "due-date", "", "Due date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.Estimate, "estimate", "", "Estimate point UUID")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "Return the native work-item response")
	if update {
		cmd.Flags().BoolVar(&f.ClearAssignees, "clear-assignees", false, "Remove all assignees")
		cmd.Flags().BoolVar(&f.ClearLabels, "clear-labels", false, "Remove all labels")
		cmd.Flags().BoolVar(&f.ClearParent, "clear-parent", false, "Clear the parent")
		cmd.Flags().BoolVar(&f.ClearDueDate, "clear-due-date", false, "Clear the due date")
		cmd.Flags().BoolVar(&f.ClearEstimate, "clear-estimate", false, "Clear the estimate")
	}
	return cmd
}
func (f issueFlags) validate(cmd *cobra.Command, update bool) error {
	if !update && strings.TrimSpace(f.Title) == "" {
		return invalid("--title is required")
	}
	if cmd.Flags().Changed("title") && strings.TrimSpace(f.Title) == "" {
		return invalid("--title cannot be empty")
	}
	if cmd.Flags().Changed("description") && f.DescriptionFile != "" {
		return invalid("--description and --description-file are mutually exclusive")
	}
	if f.Format != "markdown" && f.Format != "html" {
		return invalid("--description-format must be markdown or html")
	}
	if cmd.Flags().Changed("priority") {
		switch f.Priority {
		case "none", "urgent", "high", "medium", "low":
		default:
			return invalid("--priority must be none, urgent, high, medium or low")
		}
	}
	for _, pair := range [][2]string{{"clear-assignees", "assignee"}, {"clear-labels", "label"}, {"clear-parent", "parent"}, {"clear-due-date", "due-date"}, {"clear-estimate", "estimate"}} {
		if v, _ := cmd.Flags().GetBool(pair[0]); v && cmd.Flags().Changed(pair[1]) {
			return invalid("--" + pair[0] + " and --" + pair[1] + " are mutually exclusive")
		}
	}
	if cmd.Flags().Changed("due-date") {
		if _, err := time.Parse("2006-01-02", f.DueDate); err != nil {
			return invalid("--due-date must be YYYY-MM-DD")
		}
	}
	if cmd.Flags().Changed("estimate") {
		if _, err := uuid.Parse(f.Estimate); err != nil {
			return invalid("--estimate requires an estimate point UUID")
		}
	}
	if update {
		changed := false
		for _, name := range []string{"title", "description", "description-file", "priority", "state", "assignee", "label", "parent", "due-date", "estimate"} {
			if cmd.Flags().Changed(name) {
				changed = true
			}
		}
		changed = changed || f.ClearAssignees || f.ClearLabels || f.ClearParent || f.ClearDueDate || f.ClearEstimate
		if !changed {
			return invalid("at least one update field is required")
		}
	}
	return nil
}
func content(text, file, format string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read content file: %w", err)
		}
		text = string(data)
	}
	if format == "html" {
		return text, nil
	}
	if format != "markdown" {
		return "", invalid("content format must be markdown or html")
	}
	var out bytes.Buffer
	if err := goldmark.Convert([]byte(text), &out); err != nil {
		return "", err
	}
	return out.String(), nil
}
func (f issueFlags) input(cmd *cobra.Command, c *plane.Client, project string) (plane.IssueInput, error) {
	in := plane.IssueInput{}
	if cmd.Flags().Changed("title") {
		in.Name = plane.Set(f.Title)
	}
	if cmd.Flags().Changed("description") || f.DescriptionFile != "" {
		html, err := content(f.Description, f.DescriptionFile, f.Format)
		if err != nil {
			return in, err
		}
		in.DescriptionHTML = plane.Set(html)
	}
	if cmd.Flags().Changed("priority") {
		in.Priority = plane.Set(f.Priority)
	}
	if cmd.Flags().Changed("state") {
		id, err := resolveRelation(cmd.Context(), c, "states", project, f.State)
		if err != nil {
			return in, err
		}
		in.State = plane.Set(&id)
	}
	for _, r := range []struct {
		flag, resource string
		values         []string
		clear          bool
		field          *plane.Field[[]string]
	}{{"assignee", "members", f.Assignees, f.ClearAssignees, &in.Assignees}, {"label", "labels", f.Labels, f.ClearLabels, &in.Labels}} {
		if r.clear {
			*r.field = plane.Set([]string{})
			continue
		}
		if cmd.Flags().Changed(r.flag) {
			ids := []string{}
			seen := map[string]bool{}
			for _, ref := range r.values {
				id, err := resolveRelation(cmd.Context(), c, r.resource, project, ref)
				if err != nil {
					return in, err
				}
				if !seen[id] {
					ids = append(ids, id)
					seen[id] = true
				}
			}
			*r.field = plane.Set(ids)
		}
	}
	if f.ClearParent {
		in.Parent = plane.Set[*string](nil)
	} else if cmd.Flags().Changed("parent") {
		ref, err := c.ResolveIssue(cmd.Context(), project, f.Parent)
		if err != nil {
			return in, err
		}
		in.Parent = plane.Set(&ref.ID)
	}
	if f.ClearDueDate {
		in.TargetDate = plane.Set[*string](nil)
	} else if cmd.Flags().Changed("due-date") {
		date := f.DueDate
		in.TargetDate = plane.Set(&date)
	}
	if f.ClearEstimate {
		in.Estimate = plane.Set[*string](nil)
	} else if cmd.Flags().Changed("estimate") {
		estimate := f.Estimate
		in.Estimate = plane.Set(&estimate)
	}
	return in, nil
}

func (s *settings) commentCommand() *cobra.Command {
	root := &cobra.Command{Use: "comment", Short: "Manage work-item comments"}
	f := new(listFlags)
	list := &cobra.Command{Use: "list <issue>", Short: "List work-item comments", Args: exact(1), RunE: s.run("list Plane comments", func(cmd *cobra.Command, args []string) error {
		c, err := s.client()
		if err != nil {
			return err
		}
		q, err := f.query(cmd, c)
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		path, err := c.CommentsPath(ref)
		if err != nil {
			return err
		}
		return f.output(cmd, c, path, q)
	})}
	f.bind(list)
	root.AddCommand(list)
	var body, file, format, parent string
	var jsonOut bool
	add := &cobra.Command{Use: "add <issue>", Short: "Add a work-item comment", Args: exact(1), RunE: s.run("add Plane comment", func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("body") && file != "" {
			return invalid("--body and --body-file are mutually exclusive")
		}
		html, err := content(body, file, format)
		if err != nil {
			return err
		}
		if strings.TrimSpace(html) == "" {
			return invalid("comment content is required")
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		project, err := resolveProject(cmd.Context(), c, "", false)
		if err != nil {
			return err
		}
		ref, err := c.ResolveIssue(cmd.Context(), project, args[0])
		if err != nil {
			return err
		}
		raw, err := c.AddComment(cmd.Context(), ref, html, parent)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(cmd.OutOrStdout(), raw)
		}
		return writeRows(cmd.OutOrStdout(), []json.RawMessage{raw})
	})}
	add.Flags().StringVar(&body, "body", "", "Comment body")
	add.Flags().StringVar(&file, "body-file", "", "Read comment from a UTF-8 file")
	add.Flags().StringVar(&format, "body-format", "markdown", "Content format: markdown or html")
	add.Flags().StringVar(&parent, "reply-to", "", "Parent comment UUID")
	add.Flags().BoolVar(&jsonOut, "json", false, "Return native comment JSON")
	root.AddCommand(add)
	s.commentChanges(root)
	return root
}
func (s *settings) apiCommand() *cobra.Command {
	var input string
	cmd := &cobra.Command{Use: "api METHOD path", Short: "Make a raw Plane REST request in the selected API version", Args: exact(2), RunE: s.run("call Plane API", func(cmd *cobra.Command, args []string) error {
		var body json.RawMessage
		if input != "" {
			var data []byte
			var err error
			if input == "-" {
				data, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), 16*1024*1024+1))
			} else {
				data, err = os.ReadFile(input)
			}
			if err != nil {
				return err
			}
			if len(data) > 16*1024*1024 || !json.Valid(data) {
				return invalid("--input must contain valid JSON of at most 16 MiB")
			}
			body = data
		}
		c, err := s.client()
		if err != nil {
			return err
		}
		method := strings.ToUpper(args[0])
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPatch && method != http.MethodDelete {
			return invalid("METHOD must be GET, POST, PATCH or DELETE")
		}
		raw, err := c.RequestRaw(cmd.Context(), method, args[1], body)
		if err != nil {
			return err
		}
		return writeJSON(cmd.OutOrStdout(), raw)
	})}
	cmd.Flags().StringVar(&input, "input", "", "JSON request body file, or - for stdin")
	return cmd
}
