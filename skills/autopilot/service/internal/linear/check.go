package linear

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
)

// ErrWrongOrganization means the API key belongs to another Linear organisation.
var ErrWrongOrganization = errors.New("wrong linear organisation")

// MissingStatusesError lists every "<team key>: <status name>" pair that does not exist.
type MissingStatusesError struct {
	Missing []string
}

func (e *MissingStatusesError) Error() string {
	return fmt.Sprintf("linear statuses missing: %s", strings.Join(e.Missing, ", "))
}

// MissingProjectsError lists every "<team key>: <project name>" pair that does not exist.
type MissingProjectsError struct {
	Missing []string
}

func (e *MissingProjectsError) Error() string {
	return fmt.Sprintf("linear projects missing: %s", strings.Join(e.Missing, ", "))
}

// ProjectRef names a product project inside a team.
type ProjectRef struct {
	TeamKey string
	Name    string
}

// StartCheck is what the service checks against Linear before it starts.
type StartCheck struct {
	Workspace    string
	Statuses     map[string]string
	DefaultLabel string
	TeamKeys     []string
	Projects     []ProjectRef
}

// CheckStart refuses a wrong organisation, lists the configured statuses a team lacks, refuses a default label a
// team cannot use, and then lists the projects that do not exist. A team that several projects share is checked
// once.
func (c *Client) CheckStart(ctx context.Context, check StartCheck) error {
	viewer, err := c.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("cannot read linear viewer: %w", err)
	}
	if viewer.OrganizationKey != check.Workspace {
		return fmt.Errorf("api key organisation %q, configured linear.workspace %q: %w", viewer.OrganizationKey, check.Workspace, ErrWrongOrganization)
	}
	var missing []string
	teams := make(map[string]Team, len(check.TeamKeys))
	for _, key := range check.TeamKeys {
		if _, seen := teams[key]; seen {
			continue
		}
		team, err := c.TeamByKey(ctx, key)
		if err != nil {
			return fmt.Errorf("cannot find team %s: %w", key, err)
		}
		teams[key] = team
		states, err := c.WorkflowStates(ctx, team.ID)
		if err != nil {
			return fmt.Errorf("cannot read statuses of team %s: %w", key, err)
		}
		have := make(map[string]bool, len(states))
		for _, s := range states {
			have[s.Name] = true
		}
		for _, name := range check.Statuses {
			if !have[name] {
				missing = append(missing, fmt.Sprintf("%s: %s", key, name))
			}
		}
		if _, err := c.LabelByName(ctx, team, check.DefaultLabel); err != nil {
			return fmt.Errorf("default label of team %s: %w", key, err)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return &MissingStatusesError{Missing: missing}
	}
	var missingProjects []string
	for _, ref := range check.Projects {
		team, ok := teams[ref.TeamKey]
		if !ok {
			t, err := c.TeamByKey(ctx, ref.TeamKey)
			if err != nil {
				return fmt.Errorf("cannot find team %s: %w", ref.TeamKey, err)
			}
			team = t
			teams[ref.TeamKey] = t
		}
		_, err := c.ProjectByName(ctx, team, ref.Name)
		if errors.Is(err, domain.ErrNotFound) {
			missingProjects = append(missingProjects, fmt.Sprintf("%s: %s", ref.TeamKey, ref.Name))
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot read project %s of team %s: %w", ref.Name, ref.TeamKey, err)
		}
	}
	if len(missingProjects) > 0 {
		sort.Strings(missingProjects)
		return &MissingProjectsError{Missing: missingProjects}
	}
	return nil
}
