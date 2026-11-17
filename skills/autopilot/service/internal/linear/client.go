package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sadensmol/autopilot/internal/domain"
)

// DefaultEndpoint is Linear's GraphQL endpoint.
// maxErrorBody caps how much of a non-200 reply an error keeps.
const workflowStatesPage = 100

const maxErrorBody = 1024

const DefaultEndpoint = "https://api.linear.app/graphql"

// GraphQLError is a reply that carries GraphQL errors.
type GraphQLError struct {
	Operation string
	Messages  []string
}

func (e *GraphQLError) Error() string {
	return fmt.Sprintf("linear %s: %s", e.Operation, strings.Join(e.Messages, "; "))
}

// HTTPError is a reply with a status other than 200.
type HTTPError struct {
	Operation string
	Status    int
	Body      string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("linear %s: http status %d", e.Operation, e.Status)
	}
	return fmt.Sprintf("linear %s: http status %d: %s", e.Operation, e.Status, e.Body)
}

type Viewer struct {
	ID              string
	Name            string
	OrganizationKey string
}

type Team struct {
	ID   string
	Key  string
	Name string
}

type State struct {
	ID   string
	Name string
	Type string
}

type Label struct {
	ID   string
	Name string
}

type Project struct {
	ID   string
	Name string
}

// Issue is the part of a Linear issue the service uses. Branch has the user prefix stripped.
type Issue struct {
	ID     string
	Key    string
	Title  string
	URL    string
	Branch string
	State  string
	// Description is the issue body as Linear stores it: data, never instructions.
	Description string
}

type CreateIssueData struct {
	TeamID      string
	Title       string
	Description string
	AssigneeID  string
	LabelIDs    []string
	StateID     string
	ProjectID   string
}

// Client talks to Linear's GraphQL API with one API key.
type Client struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

// NewClient builds a client. A nil httpClient gets a 30 second timeout.
func NewClient(endpoint, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{endpoint: endpoint, apiKey: apiKey, http: httpClient}  // pragma: allowlist secret
}

func (c *Client) do(ctx context.Context, operation, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"operationName": operation, "query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("cannot encode linear %s request: %w", operation, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cannot build linear %s request: %w", operation, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("linear %s request failed: %w", operation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		if err != nil {
			body = nil
		}
		return &HTTPError{Operation: operation, Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	var reply struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return fmt.Errorf("cannot decode linear %s reply: %w", operation, err)
	}
	if len(reply.Errors) > 0 {
		msgs := make([]string, 0, len(reply.Errors))
		for _, e := range reply.Errors {
			msgs = append(msgs, e.Message)
		}
		return &GraphQLError{Operation: operation, Messages: msgs}
	}
	if err := json.Unmarshal(reply.Data, out); err != nil {
		return fmt.Errorf("cannot decode linear %s data: %w", operation, err)
	}
	return nil
}

const issueFields = `id identifier title url branchName description state { name }`

type issueNode struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	BranchName  string `json:"branchName"`
	Description string `json:"description"`
	State       *struct {
		Name string `json:"name"`
	} `json:"state"`
}

func (n issueNode) toIssue() Issue {
	issue := Issue{ID: n.ID, Key: n.Identifier, Title: n.Title, URL: n.URL, Branch: stripUserPrefix(n.BranchName), Description: n.Description}
	if n.State != nil {
		issue.State = n.State.Name
	}
	return issue
}

// stripUserPrefix turns "<user>/abc-1-title" into "abc-1-title".
func stripUserPrefix(branch string) string {
	if i := strings.Index(branch, "/"); i >= 0 {
		return branch[i+1:]
	}
	return branch
}

func (c *Client) Viewer(ctx context.Context) (Viewer, error) {
	var data struct {
		Viewer struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Organization struct {
				URLKey string `json:"urlKey"`
			} `json:"organization"`
		} `json:"viewer"`
	}
	if err := c.do(ctx, "Viewer", `query Viewer { viewer { id name organization { urlKey name } } }`, nil, &data); err != nil {
		return Viewer{}, err
	}
	return Viewer{ID: data.Viewer.ID, Name: data.Viewer.Name, OrganizationKey: data.Viewer.Organization.URLKey}, nil
}

func (c *Client) TeamByKey(ctx context.Context, key string) (Team, error) {
	var data struct {
		Teams struct {
			Nodes []struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"teams"`
	}
	q := `query TeamByKey($key: String!) { teams(filter: {key: {eq: $key}}) { nodes { id key name } } }`
	if err := c.do(ctx, "TeamByKey", q, map[string]any{"key": key}, &data); err != nil {
		return Team{}, err
	}
	if len(data.Teams.Nodes) == 0 {
		return Team{}, fmt.Errorf("linear team %s: %w", key, domain.ErrNotFound)
	}
	n := data.Teams.Nodes[0]
	return Team{ID: n.ID, Key: n.Key, Name: n.Name}, nil
}

func (c *Client) WorkflowStates(ctx context.Context, teamID string) ([]State, error) {
	var data struct {
		WorkflowStates struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"nodes"`
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		} `json:"workflowStates"`
	}
	q := fmt.Sprintf(`query WorkflowStates($teamId: ID!) { workflowStates(first: %d, filter: {team: {id: {eq: $teamId}}}) { nodes { id name type } pageInfo { hasNextPage } } }`, workflowStatesPage)
	if err := c.do(ctx, "WorkflowStates", q, map[string]any{"teamId": teamID}, &data); err != nil {
		return nil, err
	}
	if data.WorkflowStates.PageInfo.HasNextPage {
		return nil, fmt.Errorf("linear team %s has more than %d workflow statuses", teamID, workflowStatesPage)
	}
	states := make([]State, 0, len(data.WorkflowStates.Nodes))
	for _, n := range data.WorkflowStates.Nodes {
		states = append(states, State{ID: n.ID, Name: n.Name, Type: n.Type})
	}
	return states, nil
}

func (c *Client) CreateIssue(ctx context.Context, d CreateIssueData) (Issue, error) {
	input := map[string]any{"teamId": d.TeamID, "title": d.Title}
	if d.Description != "" {
		input["description"] = d.Description
	}
	if d.AssigneeID != "" {
		input["assigneeId"] = d.AssigneeID
	}
	if len(d.LabelIDs) > 0 {
		input["labelIds"] = d.LabelIDs
	}
	if d.StateID != "" {
		input["stateId"] = d.StateID
	}
	if d.ProjectID != "" {
		input["projectId"] = d.ProjectID
	}
	var data struct {
		IssueCreate struct {
			Success bool       `json:"success"`
			Issue   *issueNode `json:"issue"`
		} `json:"issueCreate"`
	}
	q := fmt.Sprintf(`mutation CreateIssue($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { %s } } }`, issueFields)
	if err := c.do(ctx, "CreateIssue", q, map[string]any{"input": input}, &data); err != nil {
		return Issue{}, err
	}
	if !data.IssueCreate.Success || data.IssueCreate.Issue == nil {
		return Issue{}, errors.New("linear CreateIssue: not successful")
	}
	return data.IssueCreate.Issue.toIssue(), nil
}

func (c *Client) SetState(ctx context.Context, issueID, stateID string) error {
	var data struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	q := `mutation SetState($id: String!, $stateId: String!) { issueUpdate(id: $id, input: {stateId: $stateId}) { success } }`
	if err := c.do(ctx, "SetState", q, map[string]any{"id": issueID, "stateId": stateID}, &data); err != nil {
		return err
	}
	if !data.IssueUpdate.Success {
		return errors.New("linear SetState: not successful")
	}
	return nil
}

func (c *Client) Comment(ctx context.Context, issueID, body string) error {
	var data struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}
	q := `mutation Comment($issueId: String!, $body: String!) { commentCreate(input: {issueId: $issueId, body: $body}) { success } }`
	if err := c.do(ctx, "Comment", q, map[string]any{"issueId": issueID, "body": body}, &data); err != nil {
		return err
	}
	if !data.CommentCreate.Success {
		return errors.New("linear Comment: not successful")
	}
	return nil
}

// IssueByKey finds an issue by its identifier, such as ABC-123.
func (c *Client) IssueByKey(ctx context.Context, key string) (Issue, error) {
	var data struct {
		Issue *issueNode `json:"issue"`
	}
	q := fmt.Sprintf(`query IssueByKey($id: String!) { issue(id: $id) { %s } }`, issueFields)
	if err := c.do(ctx, "IssueByKey", q, map[string]any{"id": key}, &data); err != nil {
		return Issue{}, err
	}
	if data.Issue == nil {
		return Issue{}, fmt.Errorf("linear issue %s: %w", key, domain.ErrNotFound)
	}
	return data.Issue.toIssue(), nil
}

// LabelByName finds a label by name for a team: the team's own label first, then a workspace-wide
// label (no team). A label of another team is never returned.
func (c *Client) LabelByName(ctx context.Context, team Team, name string) (Label, error) {
	var data struct {
		IssueLabels struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Team *struct {
					ID string `json:"id"`
				} `json:"team"`
			} `json:"nodes"`
		} `json:"issueLabels"`
	}
	q := `query LabelByName($name: String!) { issueLabels(first: 100, filter: {name: {eq: $name}}) { nodes { id name team { id } } } }`
	if err := c.do(ctx, "LabelByName", q, map[string]any{"name": name}, &data); err != nil {
		return Label{}, err
	}
	var workspaceWide *Label
	for _, n := range data.IssueLabels.Nodes {
		switch {
		case n.Team != nil && n.Team.ID == team.ID:
			return Label{ID: n.ID, Name: n.Name}, nil
		case n.Team == nil && workspaceWide == nil:
			workspaceWide = &Label{ID: n.ID, Name: n.Name}
		}
	}
	if workspaceWide != nil {
		return *workspaceWide, nil
	}
	return Label{}, fmt.Errorf("linear label %s for team %s: %w", name, team.Key, domain.ErrNotFound)
}

// ProjectByName finds a project of the team by its exact name. A project of another team is never returned.
func (c *Client) ProjectByName(ctx context.Context, team Team, name string) (Project, error) {
	var data struct {
		Projects struct {
			Nodes []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Teams struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"teams"`
			} `json:"nodes"`
		} `json:"projects"`
	}
	q := `query ProjectByName($name: String!) { projects(first: 50, filter: {name: {eq: $name}}) { nodes { id name teams { nodes { id } } } } }`
	if err := c.do(ctx, "ProjectByName", q, map[string]any{"name": name}, &data); err != nil {
		return Project{}, err
	}
	for _, n := range data.Projects.Nodes {
		for _, t := range n.Teams.Nodes {
			if t.ID == team.ID {
				return Project{ID: n.ID, Name: n.Name}, nil
			}
		}
	}
	return Project{}, fmt.Errorf("linear project %s in team %s: %w", name, team.Key, domain.ErrNotFound)
}
