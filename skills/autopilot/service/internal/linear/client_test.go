package linear_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/tests"
)

type m = map[string]any

const testKey = "lin_api_test_key"

func newClient(t *testing.T) (*linear.Client, *tests.FakeLinear) {
	fake := tests.NewFakeLinear(t)
	return linear.NewClient(fake.URL(), testKey, nil), fake
}

func reply(data any) func(map[string]any) tests.LinearReply {
	return func(map[string]any) tests.LinearReply { return tests.LinearReply{Data: data} }
}

func TestViewer(t *testing.T) {
	c, fake := newClient(t)
	fake.On("Viewer", reply(m{"viewer": m{"id": "u1", "name": "Ann", "organization": m{"urlKey": "acme", "name": "Acme"}}}))

	v, err := c.Viewer(context.Background())
	require.NoError(t, err)
	require.Equal(t, linear.Viewer{ID: "u1", Name: "Ann", OrganizationKey: "acme"}, v)
	require.Equal(t, testKey, fake.Requests()[0].Authorization)
}

func TestErrorMapping(t *testing.T) {
	ctx := context.Background()
	t.Run("graphql errors", func(t *testing.T) {
		c, fake := newClient(t)
		fake.On("Viewer", func(map[string]any) tests.LinearReply {
			return tests.LinearReply{Errors: []string{"first problem", "second problem"}}
		})
		_, err := c.Viewer(ctx)
		var gqlErr *linear.GraphQLError
		require.ErrorAs(t, err, &gqlErr)
		require.Equal(t, []string{"first problem", "second problem"}, gqlErr.Messages)
		require.Contains(t, err.Error(), "first problem")
	})
	t.Run("http status", func(t *testing.T) {
		c, fake := newClient(t)
		fake.On("Viewer", func(map[string]any) tests.LinearReply {
			return tests.LinearReply{Status: 401, Body: `{"errors":[{"message":"authentication failed"}]}`}
		})
		_, err := c.Viewer(ctx)
		var httpErr *linear.HTTPError
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, 401, httpErr.Status)
		require.Contains(t, httpErr.Body, "authentication failed")
		require.NotContains(t, err.Error(), testKey)
	})
	t.Run("unsuccessful mutation", func(t *testing.T) {
		c, fake := newClient(t)
		fake.On("SetState", reply(m{"issueUpdate": m{"success": false}}))
		require.Error(t, c.SetState(ctx, "i1", "s1"))
	})
}

func TestTeamByKey(t *testing.T) {
	ctx := context.Background()
	c, fake := newClient(t)
	fake.On("TeamByKey", func(vars map[string]any) tests.LinearReply {
		if vars["key"] == "ABC" {
			return tests.LinearReply{Data: m{"teams": m{"nodes": []m{{"id": "t1", "key": "ABC", "name": "Abc"}}}}}
		}
		return tests.LinearReply{Data: m{"teams": m{"nodes": []m{}}}}
	})
	team, err := c.TeamByKey(ctx, "ABC")
	require.NoError(t, err)
	require.Equal(t, linear.Team{ID: "t1", Key: "ABC", Name: "Abc"}, team)

	_, err = c.TeamByKey(ctx, "ZZZ")
	require.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestWorkflowStates(t *testing.T) {
	c, fake := newClient(t)
	fake.On("WorkflowStates", func(vars map[string]any) tests.LinearReply {
		require.Equal(t, "t1", vars["teamId"])
		return tests.LinearReply{Data: m{"workflowStates": m{"nodes": []m{{"id": "s1", "name": "Todo", "type": "unstarted"}, {"id": "s2", "name": "Planning", "type": "started"}}}}}
	})
	states, err := c.WorkflowStates(context.Background(), "t1")
	require.NoError(t, err)
	require.Equal(t, []linear.State{{ID: "s1", Name: "Todo", Type: "unstarted"}, {ID: "s2", Name: "Planning", Type: "started"}}, states)
}

func TestCreateIssue(t *testing.T) {
	c, fake := newClient(t)
	fake.On("CreateIssue", func(vars map[string]any) tests.LinearReply {
		in := vars["input"].(map[string]any)
		require.Equal(t, "t1", in["teamId"])
		require.Equal(t, "Fix it", in["title"])
		require.Equal(t, "body", in["description"])
		require.Equal(t, "u1", in["assigneeId"])
		require.Equal(t, []any{"l1"}, in["labelIds"])
		require.Equal(t, "s1", in["stateId"])
		return tests.LinearReply{Data: m{"issueCreate": m{"success": true, "issue": m{"id": "i1", "identifier": "ABC-123", "title": "Fix it", "url": "https://linear.example/ABC-123", "branchName": "ann/abc-123-fix-it"}}}}
	})
	issue, err := c.CreateIssue(context.Background(), linear.CreateIssueData{TeamID: "t1", Title: "Fix it", Description: "body", AssigneeID: "u1", LabelIDs: []string{"l1"}, StateID: "s1"})
	require.NoError(t, err)
	require.Equal(t, linear.Issue{ID: "i1", Key: "ABC-123", Title: "Fix it", URL: "https://linear.example/ABC-123", Branch: "abc-123-fix-it"}, issue)
}

func TestCreateIssueWithProject(t *testing.T) {
	c, fake := newClient(t)
	fake.On("CreateIssue", func(vars map[string]any) tests.LinearReply {
		require.Equal(t, "p1", vars["input"].(map[string]any)["projectId"])
		return tests.LinearReply{Data: m{"issueCreate": m{"success": true, "issue": m{"id": "i1", "identifier": "ABC-123", "title": "Fix it"}}}}
	})
	_, err := c.CreateIssue(context.Background(), linear.CreateIssueData{TeamID: "t1", Title: "Fix it", ProjectID: "p1"})
	require.NoError(t, err)
}

func TestProjectByName(t *testing.T) {
	ctx := context.Background()
	teamA := linear.Team{ID: "tA", Key: "ABC"}
	node := func(id string, teams ...string) m {
		ts := []m{}
		for _, t := range teams {
			ts = append(ts, m{"id": t})
		}
		return m{"id": id, "name": "widgets", "teams": m{"nodes": ts}}
	}
	serve := func(fake *tests.FakeLinear, nodes ...m) {
		fake.On("ProjectByName", func(vars map[string]any) tests.LinearReply {
			if vars["name"] != "widgets" {
				return tests.LinearReply{Data: m{"projects": m{"nodes": []m{}}}}
			}
			return tests.LinearReply{Data: m{"projects": m{"nodes": nodes}}}
		})
	}

	t.Run("project of the team", func(t *testing.T) {
		c, fake := newClient(t)
		serve(fake, node("pB", "tB"), node("pA", "tB", "tA"))
		project, err := c.ProjectByName(ctx, teamA, "widgets")
		require.NoError(t, err)
		require.Equal(t, linear.Project{ID: "pA", Name: "widgets"}, project)
	})
	t.Run("only another team's project is not found", func(t *testing.T) {
		c, fake := newClient(t)
		serve(fake, node("pB", "tB"))
		_, err := c.ProjectByName(ctx, teamA, "widgets")
		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "widgets")
		require.Contains(t, err.Error(), "ABC")
	})
	t.Run("no project is not found", func(t *testing.T) {
		c, fake := newClient(t)
		serve(fake)
		_, err := c.ProjectByName(ctx, teamA, "widgets")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestSetStateAndComment(t *testing.T) {
	ctx := context.Background()
	c, fake := newClient(t)
	fake.On("SetState", func(vars map[string]any) tests.LinearReply {
		require.Equal(t, "i1", vars["id"])
		require.Equal(t, "s1", vars["stateId"])
		return tests.LinearReply{Data: m{"issueUpdate": m{"success": true}}}
	})
	fake.On("Comment", func(vars map[string]any) tests.LinearReply {
		require.Equal(t, "i1", vars["issueId"])
		require.Equal(t, "hello", vars["body"])
		return tests.LinearReply{Data: m{"commentCreate": m{"success": true}}}
	})
	require.NoError(t, c.SetState(ctx, "i1", "s1"))
	require.NoError(t, c.Comment(ctx, "i1", "hello"))
}

func TestIssueByKey(t *testing.T) {
	ctx := context.Background()
	c, fake := newClient(t)
	fake.On("IssueByKey", func(vars map[string]any) tests.LinearReply {
		if vars["id"] == "ABC-123" {
			return tests.LinearReply{Data: m{"issue": m{"id": "i1", "identifier": "ABC-123", "title": "T", "url": "u", "branchName": "feature/abc-123-t", "state": m{"name": "Todo"}, "description": "the body"}}}
		}
		return tests.LinearReply{Data: m{"issue": nil}}
	})
	issue, err := c.IssueByKey(ctx, "ABC-123")
	require.NoError(t, err)
	require.Equal(t, "abc-123-t", issue.Branch)
	require.Equal(t, "Todo", issue.State)
	require.Equal(t, "the body", issue.Description)

	_, err = c.IssueByKey(ctx, "ABC-999")
	require.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestBranchWithoutPrefixIsKept(t *testing.T) {
	c, fake := newClient(t)
	fake.On("IssueByKey", reply(m{"issue": m{"id": "i1", "identifier": "ABC-1", "branchName": "abc-1-plain"}}))
	issue, err := c.IssueByKey(context.Background(), "ABC-1")
	require.NoError(t, err)
	require.Equal(t, "abc-1-plain", issue.Branch)
}

func TestLabelByName(t *testing.T) {
	ctx := context.Background()
	teamA := linear.Team{ID: "tA", Key: "ABC"}
	teamB := linear.Team{ID: "tB", Key: "DEF"}
	node := func(id, team string) m {
		n := m{"id": id, "name": "autopilot", "team": nil}
		if team != "" {
			n["team"] = m{"id": team}
		}
		return n
	}
	serve := func(nodes ...m) func(*tests.FakeLinear) {
		return func(fake *tests.FakeLinear) {
			fake.On("LabelByName", func(vars map[string]any) tests.LinearReply {
				if vars["name"] != "autopilot" {
					return tests.LinearReply{Data: m{"issueLabels": m{"nodes": []m{}}}}
				}
				return tests.LinearReply{Data: m{"issueLabels": m{"nodes": nodes}}}
			})
		}
	}
	tests2 := map[string]struct {
		setup func(*tests.FakeLinear)
		team  linear.Team
		want  string
	}{
		"same name in two teams: own team label": {setup: serve(node("lB", "tB"), node("lA", "tA")), team: teamA, want: "lA"},
		"same name in two teams: other team":     {setup: serve(node("lB", "tB"), node("lA", "tA")), team: teamB, want: "lB"},
		"team label wins over workspace label":   {setup: serve(node("lW", ""), node("lA", "tA")), team: teamA, want: "lA"},
		"workspace label when team has none":     {setup: serve(node("lB", "tB"), node("lW", "")), team: teamA, want: "lW"},
	}
	for name, tc := range tests2 {
		t.Run(name, func(t *testing.T) {
			c, fake := newClient(t)
			tc.setup(fake)
			label, err := c.LabelByName(ctx, tc.team, "autopilot")
			require.NoError(t, err)
			require.Equal(t, linear.Label{ID: tc.want, Name: "autopilot"}, label)
		})
	}

	t.Run("only another team's label is not found", func(t *testing.T) {
		c, fake := newClient(t)
		serve(node("lB", "tB"))(fake)
		_, err := c.LabelByName(ctx, teamA, "autopilot")
		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "autopilot")
		require.Contains(t, err.Error(), "ABC")
	})
	t.Run("no label is not found", func(t *testing.T) {
		c, fake := newClient(t)
		serve()(fake)
		_, err := c.LabelByName(ctx, teamA, "autopilot")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestWorkflowStatesTooManyIsError(t *testing.T) {
	c, fake := newClient(t)
	fake.On("WorkflowStates", reply(m{"workflowStates": m{"nodes": []m{{"id": "s1", "name": "Todo", "type": "unstarted"}}, "pageInfo": m{"hasNextPage": true}}}))
	_, err := c.WorkflowStates(context.Background(), "t1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than")
}

func setupCheck(fake *tests.FakeLinear, org string, statesByTeam map[string][]string) {
	fake.On("Viewer", reply(m{"viewer": m{"id": "u1", "name": "Ann", "organization": m{"urlKey": org}}}))
	fake.On("TeamByKey", func(vars map[string]any) tests.LinearReply {
		key := vars["key"].(string)
		return tests.LinearReply{Data: m{"teams": m{"nodes": []m{{"id": "id-" + key, "key": key, "name": key}}}}}
	})
	fake.On("WorkflowStates", func(vars map[string]any) tests.LinearReply {
		key := vars["teamId"].(string)[len("id-"):]
		nodes := []m{}
		for _, n := range statesByTeam[key] {
			nodes = append(nodes, m{"id": n, "name": n, "type": "started"})
		}
		return tests.LinearReply{Data: m{"workflowStates": m{"nodes": nodes}}}
	})
	fake.On("LabelByName", func(vars map[string]any) tests.LinearReply {
		return tests.LinearReply{Data: m{"issueLabels": m{"nodes": []m{{"id": "l1", "name": vars["name"], "team": nil}}}}}
	})
}

func TestCheckStart(t *testing.T) {
	ctx := context.Background()
	statuses := map[string]string{"todo": "Todo", "qa": "In QA"}

	t.Run("all good", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "acme", map[string][]string{"ABC": {"Todo", "In QA"}, "DEF": {"Todo", "In QA", "Done"}})
		require.NoError(t, c.CheckStart(ctx, linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ABC", "DEF"}}))
	})
	t.Run("wrong organisation refused", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "other", map[string][]string{"ABC": {"Todo", "In QA"}})
		err := c.CheckStart(ctx, linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ABC"}})
		require.ErrorIs(t, err, linear.ErrWrongOrganization)
		require.Contains(t, err.Error(), "acme")
		require.Contains(t, err.Error(), "other")
	})
	t.Run("missing status listed by name", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "acme", map[string][]string{"ABC": {"Todo"}, "DEF": {"In QA"}})
		err := c.CheckStart(ctx, linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ABC", "DEF"}})
		var missing *linear.MissingStatusesError
		require.ErrorAs(t, err, &missing)
		require.Equal(t, []string{"ABC: In QA", "DEF: Todo"}, missing.Missing)
		require.Contains(t, err.Error(), "In QA")
	})
	t.Run("shared team checked once", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "acme", map[string][]string{"ABC": {"Todo"}})
		err := c.CheckStart(ctx, linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ABC", "ABC"}})
		var missing *linear.MissingStatusesError
		require.ErrorAs(t, err, &missing)
		require.Equal(t, []string{"ABC: In QA"}, missing.Missing)
	})
	t.Run("product projects", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "acme", map[string][]string{"ABC": {"Todo", "In QA"}})
		fake.On("ProjectByName", func(vars map[string]any) tests.LinearReply {
			if vars["name"] != "widgets" {
				return tests.LinearReply{Data: m{"projects": m{"nodes": []m{}}}}
			}
			return tests.LinearReply{Data: m{"projects": m{"nodes": []m{{"id": "p1", "name": "widgets", "teams": m{"nodes": []m{{"id": "id-ABC"}}}}}}}}
		})
		check := linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ABC", "ABC"}}

		check.Projects = []linear.ProjectRef{{TeamKey: "ABC", Name: "widgets"}}
		require.NoError(t, c.CheckStart(ctx, check))

		check.Projects = []linear.ProjectRef{{TeamKey: "ABC", Name: "widgets"}, {TeamKey: "ABC", Name: "gadgets"}}
		err := c.CheckStart(ctx, check)
		var missing *linear.MissingProjectsError
		require.ErrorAs(t, err, &missing)
		require.Equal(t, []string{"ABC: gadgets"}, missing.Missing)
	})
	t.Run("default label checked in each team", func(t *testing.T) {
		c, fake := newClient(t)
		setupCheck(fake, "acme", map[string][]string{"ABC": {"Todo", "In QA"}, "DEF": {"Todo", "In QA"}})
		fake.On("LabelByName", reply(m{"issueLabels": m{"nodes": []m{{"id": "lA", "name": "autopilot", "team": m{"id": "id-ABC"}}}}}))
		check := linear.StartCheck{Workspace: "acme", Statuses: statuses, DefaultLabel: "autopilot", TeamKeys: []string{"ABC"}}

		require.NoError(t, c.CheckStart(ctx, check))

		check.TeamKeys = []string{"ABC", "DEF"}
		err := c.CheckStart(ctx, check)
		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "DEF")
		require.Contains(t, err.Error(), "autopilot")
	})
	t.Run("unknown team", func(t *testing.T) {
		c, fake := newClient(t)
		fake.On("Viewer", reply(m{"viewer": m{"id": "u1", "organization": m{"urlKey": "acme"}}}))
		fake.On("TeamByKey", reply(m{"teams": m{"nodes": []m{}}}))
		err := c.CheckStart(ctx, linear.StartCheck{Workspace: "acme", Statuses: statuses, TeamKeys: []string{"ZZZ"}})
		require.ErrorIs(t, err, domain.ErrNotFound)
		require.Contains(t, err.Error(), "ZZZ")
	})
}
