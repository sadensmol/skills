// Package gh reads pull requests through the gh command.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PRRef names one pull request on github.com.
type PRRef struct {
	Owner  string
	Repo   string
	Number int
}

// URL is the canonical address of the pull request; the service stores and types only this form.
func (r PRRef) URL() string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", r.Owner, r.Repo, r.Number)
}

var (
	nameOK   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	numberOK = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
)

// ParsePRURL reads https://github.com/<owner>/<repo>/pull/<number>, with an optional trailing slash.
// It refuses another scheme or host, a port, user info, a query, a fragment, an encoded character,
// a space and any extra path, so only a plain pull request address passes.
func ParsePRURL(raw string) (PRRef, error) {
	bad := fmt.Errorf("%q is not a plain pull request URL: want https://github.com/<owner>/<repo>/pull/<number>", raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil {
		return PRRef{}, bad
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "%?# \t\r\n\\") {
		return PRRef{}, bad
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" || !nameOK.MatchString(parts[0]) || !nameOK.MatchString(parts[1]) ||
		parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." || !numberOK.MatchString(parts[3]) {
		return PRRef{}, bad
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil {
		return PRRef{}, bad
	}
	return PRRef{Owner: parts[0], Repo: parts[1], Number: number}, nil
}

// Review is one review of a pull request; State is COMMENTED, CHANGES_REQUESTED, APPROVED and so on.
type Review struct {
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submittedAt"`
}

// Comment is one conversation comment, or one inline review comment.
type Comment struct {
	CreatedAt time.Time `json:"createdAt"`
}

// Check is one check run or commit status of the head commit. A check run has a name, a status, a conclusion and a
// details URL; a commit status has a context, a state and a target URL.
type Check struct {
	Name        string    `json:"name"`
	Context     string    `json:"context"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	State       string    `json:"state"`
	DetailsURL  string    `json:"detailsUrl"`
	TargetURL   string    `json:"targetUrl"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

// Label is the name of a check run, or the context of a commit status.
func (c Check) Label() string {
	return firstSet(c.Name, c.Context)
}

// Link is the page of the check.
func (c Check) Link() string {
	return firstSet(c.DetailsURL, c.TargetURL)
}

// Pending is true while a check has no result: a check run until it completes, a commit status while it is pending
// or expected.
func (c Check) Pending() bool {
	if c.Status != "" {
		return !strings.EqualFold(c.Status, "COMPLETED")
	}
	switch strings.ToUpper(c.State) {
	case "", "PENDING", "EXPECTED":
		return true
	}
	return false
}

// Passed is true for a check that ended well; a neutral or skipped check counts as passed.
func (c Check) Passed() bool {
	switch strings.ToUpper(firstSet(c.Conclusion, c.State)) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return true
	}
	return false
}

// Failed is true for a check that ended badly. A check run reports a conclusion, a commit status a state.
func (c Check) Failed() bool {
	switch strings.ToUpper(firstSet(c.Conclusion, c.State)) {
	case "FAILURE", "ERROR", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED":
		return true
	}
	return false
}

// At is when the check ended; a commit status has only the time it started.
func (c Check) At() time.Time {
	if !c.CompletedAt.IsZero() {
		return c.CompletedAt
	}
	return c.StartedAt
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// PullRequest is what `gh pr view` reports.
type PullRequest struct {
	State       string      `json:"state"`
	HeadRefName string      `json:"headRefName"`
	BaseRefName string      `json:"baseRefName"`
	CreatedAt   time.Time   `json:"createdAt"`
	Reviews     []Review    `json:"reviews"`
	Comments    []Comment   `json:"comments"`
	Checks      []Check     `json:"statusCheckRollup"`
	MergeCommit MergeCommit `json:"mergeCommit"`
}

// MergeCommit is the commit a merge made; it is empty while the pull request is open.
type MergeCommit struct {
	OID string `json:"oid"`
}

// ReviewComment is one inline comment on the diff; `gh pr view` does not list these.
type ReviewComment struct {
	CreatedAt time.Time `json:"created_at"`
}

// Client calls gh; gh keeps its own login, so the service holds no token.
// Every call has a time limit: a hung gh is killed, so one PR cannot stall the watch of the others.
type Client struct {
	bin     string
	timeout time.Duration
}

func New(bin string, timeout time.Duration) *Client {
	return &Client{bin: bin, timeout: timeout}
}

// View reads the state, branches, merge commit, reviews, comments and checks of a pull request.
func (c *Client) View(ctx context.Context, prURL string) (PullRequest, error) {
	out, err := c.run(ctx, "pr", "view", prURL, "--json", "state,headRefName,baseRefName,headRefOid,mergeCommit,createdAt,reviews,comments,statusCheckRollup")
	if err != nil {
		return PullRequest{}, err
	}
	var pr PullRequest
	if err := json.Unmarshal(out, &pr); err != nil {
		return PullRequest{}, fmt.Errorf("cannot read the reply of gh pr view: %w", err)
	}
	return pr, nil
}

// ReviewComments lists the inline comments of a pull request, every page of them.
func (c *Client) ReviewComments(ctx context.Context, ref PRRef) ([]ReviewComment, error) {
	out, err := c.run(ctx, "api", "--paginate", fmt.Sprintf("repos/%s/%s/pulls/%d/comments", ref.Owner, ref.Repo, ref.Number))
	if err != nil {
		return nil, err
	}
	// gh prints one JSON array per page
	var comments []ReviewComment
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var page []ReviewComment
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("cannot read the reply of gh api: %w", err)
		}
		comments = append(comments, page...)
	}
	return comments, nil
}

// MergeMethod is the gh flag of the first merge method the repo allows: a merge commit, a squash, a rebase.
func (c *Client) MergeMethod(ctx context.Context, ref PRRef) (string, error) {
	out, err := c.run(ctx, "api", fmt.Sprintf("repos/%s/%s", ref.Owner, ref.Repo))
	if err != nil {
		return "", err
	}
	var repo struct {
		MergeCommit bool `json:"allow_merge_commit"`
		Squash      bool `json:"allow_squash_merge"`
		Rebase      bool `json:"allow_rebase_merge"`
	}
	if err := json.Unmarshal(out, &repo); err != nil {
		return "", fmt.Errorf("cannot read the reply of gh api: %w", err)
	}
	switch {
	case repo.MergeCommit:
		return "--merge", nil
	case repo.Squash:
		return "--squash", nil
	case repo.Rebase:
		return "--rebase", nil
	}
	return "", fmt.Errorf("repo %s/%s allows no merge method", ref.Owner, ref.Repo)
}

// Merge merges a pull request with the method flag and deletes its branch on GitHub.
func (c *Client) Merge(ctx context.Context, prURL, method string) error {
	_, err := c.run(ctx, "pr", "merge", prURL, method, "--delete-branch")
	return err
}

// DeleteBranch deletes a branch on GitHub; a branch that is gone already counts as deleted.
func (c *Client) DeleteBranch(ctx context.Context, ref PRRef, branch string) error {
	_, err := c.run(ctx, "api", "-X", "DELETE", fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", ref.Owner, ref.Repo, branch))
	if err != nil && !strings.Contains(err.Error(), "(HTTP 422)") && !strings.Contains(err.Error(), "(HTTP 404)") {
		return err
	}
	return nil
}

// Job is one CI job of a workflow run; Conclusion is set once Status is completed.
type Job struct {
	Status     string      `json:"status"`
	Conclusion string      `json:"conclusion"`
	URL        string      `json:"url"`
	Run        WorkflowRun `json:"-"`
}

// WorkflowRun is the workflow run that holds a job. Failed names its jobs that ended with a failure, a cancel or a
// timeout.
type WorkflowRun struct {
	ID     int64
	URL    string
	Failed []string
}

// JobOnCommit finds the job with the name in the workflow runs of a commit of repo (owner/name), the newest run
// first. It reports false while no run of the commit has the job.
func (c *Client) JobOnCommit(ctx context.Context, repo, sha, job string) (Job, bool, error) {
	out, err := c.run(ctx, "run", "list", "--repo", repo, "--commit", sha, "--json", "databaseId")
	if err != nil {
		return Job{}, false, err
	}
	var runs []struct {
		ID int64 `json:"databaseId"`
	}
	if err := json.Unmarshal(out, &runs); err != nil {
		return Job{}, false, fmt.Errorf("cannot read the reply of gh run list: %w", err)
	}
	for _, r := range runs {
		out, err := c.run(ctx, "run", "view", strconv.FormatInt(r.ID, 10), "--repo", repo, "--json", "jobs,url")
		if err != nil {
			return Job{}, false, err
		}
		var view struct {
			URL  string `json:"url"`
			Jobs []struct {
				Name string `json:"name"`
				Job
			} `json:"jobs"`
		}
		if err := json.Unmarshal(out, &view); err != nil {
			return Job{}, false, fmt.Errorf("cannot read the reply of gh run view: %w", err)
		}
		run := WorkflowRun{ID: r.ID, URL: view.URL}
		for _, j := range view.Jobs {
			switch j.Conclusion {
			case "failure", "cancelled", "timed_out":
				run.Failed = append(run.Failed, j.Name)
			}
		}
		for _, j := range view.Jobs {
			if j.Name == job {
				j.Job.Run = run
				return j.Job, true, nil
			}
		}
	}
	return Job{}, false, nil
}

// RerunFailed re-runs the failed jobs of a workflow run of repo (owner/name), and the jobs that need them.
func (c *Client) RerunFailed(ctx context.Context, repo string, runID int64) error {
	_, err := c.run(ctx, "run", "rerun", strconv.FormatInt(runID, 10), "--failed", "--repo", repo)
	return err
}

// BranchHead is the commit at the head of a branch of repo (owner/name).
func (c *Client) BranchHead(ctx context.Context, repo, branch string) (string, error) {
	out, err := c.run(ctx, "api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch))
	if err != nil {
		return "", err
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(out, &ref); err != nil {
		return "", fmt.Errorf("cannot read the reply of gh api: %w", err)
	}
	if ref.Object.SHA == "" {
		return "", fmt.Errorf("branch %s of %s has no head commit", branch, repo)
	}
	return ref.Object.SHA, nil
}

// Contains is true when the commit sha of repo (owner/name) is the commit ancestor or one after it.
func (c *Client) Contains(ctx context.Context, repo, sha, ancestor string) (bool, error) {
	out, err := c.run(ctx, "api", fmt.Sprintf("repos/%s/compare/%s...%s", repo, ancestor, sha))
	if err != nil {
		return false, err
	}
	var compare struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &compare); err != nil {
		return false, fmt.Errorf("cannot read the reply of gh api: %w", err)
	}
	return compare.Status == "ahead" || compare.Status == "identical", nil
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, args...)
	// a child that outlives the kill must not hold the output pipes open
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %s: %w", strings.Join(args[:min(2, len(args))], " "), strings.TrimSpace(stderr.String()), err)
	}
	return stdout.Bytes(), nil
}
