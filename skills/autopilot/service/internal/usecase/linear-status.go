package usecase

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
)

// IMirrorLinear is the part of the Linear client that status moves use.
type IMirrorLinear interface {
	IssueByKey(ctx context.Context, key string) (linear.Issue, error)
	TeamByKey(ctx context.Context, key string) (linear.Team, error)
	WorkflowStates(ctx context.Context, teamID string) ([]linear.State, error)
	SetState(ctx context.Context, issueID, stateID string) error
}

// linearMirror maps the phases of a run to the Linear statuses of its team.
type linearMirror struct {
	linear   IMirrorLinear
	registry config.Registry
}

// set moves the issue of the run to the status of the phase.
func (m linearMirror) set(ctx context.Context, run domain.Run, phase string) error {
	name, ok := m.registry.Linear.Statuses[phase]
	if !ok {
		return fmt.Errorf("no linear status is configured for phase %s", phase)
	}
	issue, states, err := m.issueAndStates(ctx, run)
	if err != nil {
		return err
	}
	for _, st := range states {
		if st.Name == name {
			return m.linear.SetState(ctx, issue.ID, st.ID)
		}
	}
	return fmt.Errorf("team of run %s has no status %s", run.IssueKey, name)
}

// current returns the name of the status the issue has in Linear now.
func (m linearMirror) current(ctx context.Context, run domain.Run) (string, error) {
	issue, err := m.linear.IssueByKey(ctx, string(run.IssueKey))
	if err != nil {
		return "", fmt.Errorf("cannot read issue %s: %w", run.IssueKey, err)
	}
	return issue.State, nil
}

func (m linearMirror) issueAndStates(ctx context.Context, run domain.Run) (linear.Issue, []linear.State, error) {
	project, err := runProject(m.registry, run)
	if err != nil {
		return linear.Issue{}, nil, err
	}
	teamKey := project.LinearTeam
	issue, err := m.linear.IssueByKey(ctx, string(run.IssueKey))
	if err != nil {
		return linear.Issue{}, nil, fmt.Errorf("cannot read issue %s: %w", run.IssueKey, err)
	}
	team, err := m.linear.TeamByKey(ctx, teamKey)
	if err != nil {
		return linear.Issue{}, nil, fmt.Errorf("cannot find team %s: %w", teamKey, err)
	}
	states, err := m.linear.WorkflowStates(ctx, team.ID)
	if err != nil {
		return linear.Issue{}, nil, fmt.Errorf("cannot read statuses of team %s: %w", teamKey, err)
	}
	return issue, states, nil
}

// setDone moves the issue of a run to done unless it is done already.
func (m linearMirror) setDone(ctx context.Context, run domain.Run) error {
	current, err := m.current(ctx, run)
	if err != nil {
		return err
	}
	if current == m.registry.Linear.Statuses[donePhase] {
		return nil
	}
	if err := m.set(ctx, run, donePhase); err != nil {
		return fmt.Errorf("cannot set the linear status of run %s to done: %w", run.IssueKey, err)
	}
	return nil
}
