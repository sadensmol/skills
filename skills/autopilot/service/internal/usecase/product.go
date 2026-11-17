package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

// ProductError says what a project of the registry lacks. Reason names it and carries no local path, so it can be
// posted to Slack.
type ProductError struct {
	Product string
	Reason  string
}

func (e *ProductError) Error() string {
	return fmt.Sprintf("product %s: %s", e.Product, e.Reason)
}

// runProject returns the registry project of the product of a run. A project removed from the registry after the
// run started is an error that names both.
func runProject(registry config.Registry, run domain.Run) (config.Project, error) {
	p, ok := registry.Project(run.Product)
	if !ok {
		return config.Project{}, fmt.Errorf("product %s of run %s is not in the workspace registry", run.Product, run.IssueKey)
	}
	return p, nil
}

// IProductLinear finds the Linear team and the Linear project of a project.
type IProductLinear interface {
	TeamByKey(ctx context.Context, key string) (linear.Team, error)
	ProjectByName(ctx context.Context, team linear.Team, name string) (linear.Project, error)
}

// ProductReport is the check of every part of one project. Each error is a *ProductError when the part is
// missing, another error when the check itself failed.
type ProductReport struct {
	Project   config.Project
	Team      linear.Team
	ProjectID string
	// ChannelErr is set by CheckProducts only: intake gets messages from member channels alone.
	ChannelErr error
	LinearErr  error
	// RepoErrs has one entry per repo of the project, nil when the repo is ok.
	RepoErrs []error
}

// InspectProduct checks the Linear team of a project and its Linear project when it names one, and that each repo
// folder is a git checkout.
func InspectProduct(ctx context.Context, lin IProductLinear, repoPath func(repo string) string, p config.Project) ProductReport {
	report := ProductReport{Project: p}
	report.Team, report.ProjectID, report.LinearErr = linearProject(ctx, lin, p)
	for _, r := range p.Repos {
		report.RepoErrs = append(report.RepoErrs, repoCheckout(p.SlackChannel, r.Name, repoPath(r.Name)))
	}
	return report
}

func linearProject(ctx context.Context, lin IProductLinear, p config.Project) (linear.Team, string, error) {
	team, err := lin.TeamByKey(ctx, p.LinearTeam)
	if errors.Is(err, domain.ErrNotFound) {
		return linear.Team{}, "", &ProductError{Product: p.SlackChannel, Reason: fmt.Sprintf("no Linear team `%s`", p.LinearTeam)}
	}
	if err != nil {
		return linear.Team{}, "", fmt.Errorf("cannot find linear team %s: %w", p.LinearTeam, err)
	}
	if p.LinearProject == "" {
		return team, "", nil
	}
	project, err := lin.ProjectByName(ctx, team, p.LinearProject)
	if errors.Is(err, domain.ErrNotFound) {
		return team, "", &ProductError{Product: p.SlackChannel, Reason: fmt.Sprintf("no Linear project `%s` in team %s", p.LinearProject, p.LinearTeam)}
	}
	if err != nil {
		return team, "", fmt.Errorf("cannot find linear project %s: %w", p.LinearProject, err)
	}
	return team, project.ID, nil
}

// repoCheckout accepts a .git folder or file, so a worktree counts as a checkout.
func repoCheckout(product, repo, dir string) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return &ProductError{Product: product, Reason: fmt.Sprintf("no repo folder for `%s`", repo)}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return &ProductError{Product: product, Reason: fmt.Sprintf("repo folder for `%s` is not a git checkout", repo)}
	}
	return nil
}

// Err returns the first failed check, else one *ProductError that lists every missing part, else nil.
func (r ProductReport) Err() error {
	var reasons []string
	for _, err := range append([]error{r.ChannelErr, r.LinearErr}, r.RepoErrs...) {
		if err == nil {
			continue
		}
		var missing *ProductError
		if !errors.As(err, &missing) {
			return err
		}
		reasons = append(reasons, missing.Reason)
	}
	if len(reasons) == 0 {
		return nil
	}
	return &ProductError{Product: r.Project.SlackChannel, Reason: strings.Join(reasons, "; ")}
}

// Line is one line for a person: the channel, the Linear project (or "project: none" when the project names no
// Linear project) and each repo, each ok or not. It carries no local path and no secret.
func (r ProductReport) Line() string {
	parts := []string{fmt.Sprintf("channel %s", checkWord(r.ChannelErr))}
	switch {
	case r.Project.LinearProject != "":
		parts = append(parts, fmt.Sprintf("project %s %s", r.Project.LinearProject, checkWord(r.LinearErr)))
	case r.LinearErr != nil:
		parts = append(parts, fmt.Sprintf("team %s %s", r.Project.LinearTeam, checkWord(r.LinearErr)))
	default:
		parts = append(parts, "project: none")
	}
	for n, repo := range r.Project.Repos {
		parts = append(parts, fmt.Sprintf("repo %s %s", repo.Name, checkWord(r.RepoErrs[n])))
	}
	return fmt.Sprintf("#%s: %s", r.Project.SlackChannel, strings.Join(parts, "; "))
}

// checkWord is ok, missing (reason) for a missing part, or error (text) when the check itself failed.
func checkWord(err error) string {
	var missing *ProductError
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &missing):
		return fmt.Sprintf("missing (%s)", missing.Reason)
	default:
		return fmt.Sprintf("error (%s)", err)
	}
}

type CheckProductsData struct {
	Registry config.Registry
	RepoPath func(repo string) string
	Slack    IChannels
	Linear   IProductLinear
}

// CheckProducts checks every project of the registry: its Slack channel exists and has the bot as a member, its
// Linear team exists and so does its Linear project when it names one, and each repo folder is a git checkout.
func CheckProducts(ctx context.Context, d CheckProductsData) ([]ProductReport, error) {
	channels, err := projectChannels(ctx, d.Slack, d.Registry.Projects)
	if err != nil {
		return nil, err
	}
	reports := make([]ProductReport, 0, len(d.Registry.Projects))
	for n, p := range d.Registry.Projects {
		report := InspectProduct(ctx, d.Linear, d.RepoPath, p)
		report.ChannelErr = channels[n].Err
		reports = append(reports, report)
	}
	return reports, nil
}

// projectChannel is the Slack channel of one project. Err is a *ProductError when the channel is missing or does
// not have the bot as a member; ID is then empty.
type projectChannel struct {
	ID  string
	Err error
}

// projectChannels lists the Slack channels once and matches each project to its channel by name, in the order of
// projects.
func projectChannels(ctx context.Context, slack IChannels, projects []config.Project) ([]projectChannel, error) {
	channels, err := slack.Channels(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]slackpost.Channel, len(channels))
	for _, c := range channels {
		byName[c.Name] = c
	}
	out := make([]projectChannel, 0, len(projects))
	for _, p := range projects {
		c, ok := byName[p.SlackChannel]
		switch {
		case !ok:
			out = append(out, projectChannel{Err: &ProductError{Product: p.SlackChannel, Reason: fmt.Sprintf("no Slack channel `#%s`", p.SlackChannel)}})
		case !c.IsMember:
			out = append(out, projectChannel{Err: &ProductError{Product: p.SlackChannel, Reason: fmt.Sprintf("the bot is not a member of `#%s`", p.SlackChannel)}})
		default:
			out = append(out, projectChannel{ID: c.ID})
		}
	}
	return out, nil
}
