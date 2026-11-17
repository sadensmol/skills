package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultDeployTimeout is how long after a merge the service waits for the deploy of a repo when its registry
// entry names no timeout_minutes.
const defaultDeployTimeout = 60 * time.Minute

// phases are the run phases that need a Linear status name.
var phases = []string{"todo", "planning", "implementing", "qa", "review", "pr", "blocked", "done", "canceled"}

// Registry is the workspace registry in <workspace folder>/.workspace/: settings.json holds the workspace data,
// projects/<project>.json one project each. A project file may hold other keys, tokens and secrets among them;
// they are never read.
type Registry struct {
	Company         string
	AgtermWorkspace string
	Slack           struct {
		Workspace string
	}
	Linear struct {
		Workspace string
		// Team is optional: the shared team of the workspace, when it has one.
		Team         string
		Statuses     map[string]string
		DefaultLabel string
	}
	Projects []Project
}

// Project is one project autopilot serves: its Slack channel, its Linear team and its repos.
type Project struct {
	SlackChannel string
	LinearTeam   string
	// LinearProject is the Linear project of LinearTeam that new issues go to; it is required only when
	// several projects share LinearTeam. Empty: new issues get no Linear project.
	LinearProject string
	Repos         []Repo
}

// Repo is one repository of a project.
type Repo struct {
	Name          string
	DefaultBranch string
	// ProfileOverrides come from a JSON object of strings, numbers and booleans, each kept as text.
	ProfileOverrides map[string]string
	Deploy           RepoDeploy
}

// RepoDeploy says how a merge of the repo goes live: Manual alone, or Job and Check (one of them at least).
type RepoDeploy struct {
	// Job is the CI job on the merge commit that publishes the artifact.
	Job string
	// Check is a shell command that exits 0 when the merge commit is live; {sha} and {short_sha} are replaced first.
	Check string
	// Manual says how a person deploys the repo; the service deploys nothing then.
	Manual string
	// Timeout is how long after the merge the service waits for Job and Check.
	Timeout time.Duration
}

// Project returns the project of a Slack channel name.
func (r Registry) Project(channel string) (Project, bool) {
	for _, p := range r.Projects {
		if p.SlackChannel == channel {
			return p, true
		}
	}
	return Project{}, false
}

// Repo returns the repo of the project with the name.
func (p Project) Repo(name string) (Repo, bool) {
	for _, r := range p.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

// HasRepo reports whether the project has a repo with the name.
func (p Project) HasRepo(name string) bool {
	for _, r := range p.Repos {
		if r.Name == name {
			return true
		}
	}
	return false
}

// LoadRegistry reads the registry of a workspace folder. registry.py of the project skill owns the schema: the
// loader asks for the same keys, by their exact names, and also needs a string where it reads one.
func LoadRegistry(workspaceFolder string) (Registry, error) {
	path := filepath.Join(workspaceFolder, ".workspace", "settings.json")
	settings, err := readObject(path)
	if err != nil {
		return Registry{}, err
	}
	r, err := registryOf(settings)
	if err != nil {
		return Registry{}, err
	}
	dir := filepath.Join(workspaceFolder, ".workspace", "projects")
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return Registry{}, fmt.Errorf("cannot list %s: %w", dir, err)
	}
	if len(files) == 0 {
		return Registry{}, fmt.Errorf("no project file in %s", dir)
	}
	fileOf := make(map[string]string, len(files))
	for _, file := range files {
		obj, err := readObject(file)
		if err != nil {
			return Registry{}, err
		}
		p, err := projectOf(obj)
		if err != nil {
			return Registry{}, err
		}
		if other, ok := fileOf[p.SlackChannel]; ok {
			return Registry{}, fmt.Errorf("slack_channel %s is in both %s and %s", p.SlackChannel, other, file)
		}
		fileOf[p.SlackChannel] = file
		r.Projects = append(r.Projects, p)
	}
	if err := r.validateSharedTeams(fileOf); err != nil {
		return Registry{}, err
	}
	return r, nil
}

// validateSharedTeams requires linear_project of each project whose team another project uses too: the project
// is what tells their issues apart.
func (r Registry) validateSharedTeams(fileOf map[string]string) error {
	users := map[string]int{}
	for _, p := range r.Projects {
		users[p.LinearTeam]++
	}
	for _, p := range r.Projects {
		if users[p.LinearTeam] > 1 && p.LinearProject == "" {
			return fmt.Errorf("key linear_project is missing in %s: team %s is shared, so linear_project is required", fileOf[p.SlackChannel], p.LinearTeam)
		}
	}
	return nil
}

func registryOf(o jsonObject) (Registry, error) {
	var r Registry
	if _, ok := o.keys["products"]; ok {
		return Registry{}, fmt.Errorf("key products is not read in %s: move each project to projects/<project>.json", o.path)
	}
	slack, err := o.object("slack")
	if err != nil {
		return Registry{}, err
	}
	lin, err := o.object("linear")
	if err != nil {
		return Registry{}, err
	}
	for _, k := range []struct {
		obj  jsonObject
		name string
		out  *string
	}{
		{obj: o, name: "company", out: &r.Company},
		{obj: o, name: "agterm_workspace", out: &r.AgtermWorkspace},
		{obj: slack, name: "workspace", out: &r.Slack.Workspace},
		{obj: lin, name: "workspace", out: &r.Linear.Workspace},
		{obj: lin, name: "default_label", out: &r.Linear.DefaultLabel},
	} {
		if *k.out, err = k.obj.required(k.name); err != nil {
			return Registry{}, err
		}
	}
	if r.Linear.Team, err = lin.str("team"); err != nil {
		return Registry{}, err
	}
	statuses, err := lin.object("statuses")
	if err != nil {
		return Registry{}, err
	}
	if r.Linear.Statuses, err = statusesOf(statuses); err != nil {
		return Registry{}, err
	}
	return r, nil
}

func statusesOf(o jsonObject) (map[string]string, error) {
	known := map[string]bool{}
	for _, ph := range phases {
		known[ph] = true
	}
	var unknown []string
	for ph := range o.keys {
		if !known[ph] {
			unknown = append(unknown, ph)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("key linear.statuses.%s is not a phase in %s", strings.Join(unknown, ", "), o.path)
	}
	out := make(map[string]string, len(phases))
	for _, ph := range phases {
		name, err := o.required(ph)
		if err != nil {
			return nil, err
		}
		out[ph] = name
	}
	return out, nil
}

func projectOf(o jsonObject) (Project, error) {
	var p Project
	var err error
	if p.SlackChannel, err = o.required("slack_channel"); err != nil {
		return Project{}, err
	}
	if p.LinearTeam, err = o.required("linear_team"); err != nil {
		return Project{}, err
	}
	if p.LinearProject, err = o.str("linear_project"); err != nil {
		return Project{}, err
	}
	repos, err := o.list("repos")
	if err != nil {
		return Project{}, err
	}
	if len(repos) == 0 {
		return Project{}, fmt.Errorf("key repos has no repo in %s", o.path)
	}
	for _, ro := range repos {
		r, err := repoOf(ro)
		if err != nil {
			return Project{}, err
		}
		p.Repos = append(p.Repos, r)
	}
	return p, nil
}

func repoOf(o jsonObject) (Repo, error) {
	var r Repo
	var err error
	if r.Name, err = o.required("name"); err != nil {
		return Repo{}, err
	}
	if r.DefaultBranch, err = o.required("default_branch"); err != nil {
		return Repo{}, err
	}
	if r.Deploy, err = deployOf(o); err != nil {
		return Repo{}, fmt.Errorf("repo %s: %w", r.Name, err)
	}
	overrides, err := o.object("profile_overrides")
	if err != nil || overrides.keys == nil {
		return r, err
	}
	r.ProfileOverrides = make(map[string]string, len(overrides.keys))
	for k, raw := range overrides.keys {
		v, ok := scalarText(raw)
		if !ok {
			return Repo{}, fmt.Errorf("key %s must be a string, number or boolean in %s", overrides.key(k), o.path)
		}
		r.ProfileOverrides[k] = v
	}
	return r, nil
}

// deployOf reads the required deploy object of a repo.
func deployOf(repo jsonObject) (RepoDeploy, error) {
	if raw, ok := repo.keys["deploy"]; !ok || isNull(raw) {
		return RepoDeploy{}, fmt.Errorf("key %s is missing in %s", repo.key("deploy"), repo.path)
	}
	o, err := repo.object("deploy")
	if err != nil {
		return RepoDeploy{}, err
	}
	d := RepoDeploy{Timeout: defaultDeployTimeout}
	for _, k := range []struct {
		name string
		out  *string
	}{
		{name: "job", out: &d.Job},
		{name: "check", out: &d.Check},
		{name: "manual", out: &d.Manual},
	} {
		if *k.out, err = o.str(k.name); err != nil {
			return RepoDeploy{}, err
		}
	}
	if (d.Manual != "") == (d.Job != "" || d.Check != "") {
		return RepoDeploy{}, fmt.Errorf("key %s needs manual alone, or job or check (without manual) in %s", o.prefix, o.path)
	}
	if raw, ok := o.keys["timeout_minutes"]; ok && !isNull(raw) {
		var minutes int
		if err := json.Unmarshal(raw, &minutes); err != nil || minutes <= 0 {
			return RepoDeploy{}, fmt.Errorf("key %s must be a whole number above zero in %s", o.key("timeout_minutes"), o.path)
		}
		d.Timeout = time.Duration(minutes) * time.Minute
	}
	return d, nil
}

// scalarText is the text of a JSON string, number or boolean; a number keeps its JSON spelling.
func scalarText(raw json.RawMessage) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	default:
		return "", false
	}
}

// jsonObject is one JSON object of a registry file. It reads keys by their exact names, as registry.py does
// (encoding/json would match a struct field case-insensitively), and never keeps a key it is not asked for.
// Error text names the file and the key, never a value.
type jsonObject struct {
	path string
	// prefix is the key path of the object in its file, empty at the top.
	prefix string
	keys   map[string]json.RawMessage
}

// readObject reads a JSON file whose top level is an object.
func readObject(path string) (jsonObject, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return jsonObject{}, fmt.Errorf("file %s is missing", path)
		}
		return jsonObject{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	o := jsonObject{path: path}
	err = json.Unmarshal(raw, &o.keys)
	var syntaxErr *json.SyntaxError
	switch {
	case err == nil && o.keys != nil:
		return o, nil
	case errors.As(err, &syntaxErr):
		return jsonObject{}, fmt.Errorf("%s is not valid JSON at byte %d", path, syntaxErr.Offset)
	default:
		return jsonObject{}, fmt.Errorf("%s is not a JSON object", path)
	}
}

func (o jsonObject) key(name string) string {
	if o.prefix == "" {
		return name
	}
	return fmt.Sprintf("%s.%s", o.prefix, name)
}

func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// str reads a string key; a missing key and JSON null are "".
func (o jsonObject) str(name string) (string, error) {
	raw, ok := o.keys[name]
	if !ok || isNull(raw) {
		return "", nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("key %s must be a string in %s", o.key(name), o.path)
	}
	return v, nil
}

// required reads a string key that must not be empty.
func (o jsonObject) required(name string) (string, error) {
	v, err := o.str(name)
	if err == nil && v == "" {
		return "", fmt.Errorf("key %s is missing in %s", o.key(name), o.path)
	}
	return v, err
}

// object reads an object key; a missing key and JSON null are an object with no keys.
func (o jsonObject) object(name string) (jsonObject, error) {
	child := jsonObject{path: o.path, prefix: o.key(name)}
	raw, ok := o.keys[name]
	if !ok || isNull(raw) {
		return child, nil
	}
	if err := json.Unmarshal(raw, &child.keys); err != nil {
		return jsonObject{}, fmt.Errorf("key %s must be an object in %s", child.prefix, o.path)
	}
	return child, nil
}

// list reads a list of objects; a missing key and JSON null are an empty list.
func (o jsonObject) list(name string) ([]jsonObject, error) {
	raw, ok := o.keys[name]
	if !ok || isNull(raw) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("key %s must be a list in %s", o.key(name), o.path)
	}
	out := make([]jsonObject, 0, len(items))
	for i, item := range items {
		child := jsonObject{path: o.path, prefix: fmt.Sprintf("%s[%d]", o.key(name), i)}
		if err := json.Unmarshal(item, &child.keys); err != nil || child.keys == nil {
			return nil, fmt.Errorf("key %s must be an object in %s", child.prefix, o.path)
		}
		out = append(out, child)
	}
	return out, nil
}
