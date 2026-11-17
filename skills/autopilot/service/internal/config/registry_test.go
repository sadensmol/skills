package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadensmol/autopilot/internal/config"
)

const projectSecret = "tok-very-secret-value"

func validRegistrySettings() map[string]any {
	return map[string]any{
		"company":          "acme",
		"agterm_workspace": "acme",
		"slack":            map[string]any{"workspace": "acme"},
		"linear": map[string]any{
			"workspace":     "acme",
			"team":          "ABC",
			"default_label": "autopilot",
			"statuses": map[string]any{
				"todo": "Todo", "planning": "Planning", "implementing": "In Progress", "qa": "QA", "review": "Review",
				"pr": "In Review", "blocked": "Blocked", "done": "Done", "canceled": "Canceled",
			},
		},
	}
}

func validProject(channel string) map[string]any {
	return map[string]any{
		"slack_channel":  channel,
		"linear_team":    "ABC",
		"linear_project": channel,
		"repos": []any{map[string]any{"name": channel + "-api", "default_branch": "main",
			"deploy": map[string]any{"job": "deploy", "check": "curl -fs https://example.com/version | grep -q {short_sha}"}}},
	}
}

// writeRegistry writes <folder>/.workspace/settings.json and one projects/<name>.json per project, and returns
// the workspace folder.
func writeRegistry(t *testing.T, settings map[string]any, projects map[string]map[string]any) string {
	t.Helper()
	folder := t.TempDir()
	dir := filepath.Join(folder, ".workspace", "projects")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(folder, ".workspace", "settings.json"), settings)
	for name, p := range projects {
		writeJSON(t, filepath.Join(dir, name+".json"), p)
	}
	return folder
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(raw), 0o600)
}

type registryCase struct {
	settings func(s map[string]any)
	project  func(p map[string]any)
	// shop, when set, is a second project file, shop.json.
	shop     map[string]any
	wantErr  string
	wantFile string
	// stricter marks a file that registry.py check accepts and the service refuses: registry.py checks the keys,
	// and the service also needs a string where it reads a string.
	stricter bool
}

// registryCases are registries that registry.py check accepts (no wantErr) or refuses; the Go loader must agree,
// except in a stricter case.
func registryCases() map[string]registryCase {
	noProject := validProject("shop")
	delete(noProject, "linear_project")
	return map[string]registryCase{
		"valid":                {},
		"no settings team":     {settings: func(s map[string]any) { delete(linearOf(s), "team") }},
		"unknown settings":     {settings: func(s map[string]any) { s["extra"] = map[string]any{"k": projectSecret} }},
		"own team, no project": {project: func(p map[string]any) { delete(p, "linear_project") }},
		"shared team":          {shop: validProject("shop")},
		"scalar overrides": {project: func(p map[string]any) {
			repoOf(p)["profile_overrides"] = map[string]any{"diff_coverage_min": 70, "mutation_min": 55.5, "test": "make test", "x": true}
		}},
		"mixed-case products": {settings: func(s map[string]any) { s["Products"] = []any{} }},
		"manual deploy":       {project: func(p map[string]any) { repoOf(p)["deploy"] = map[string]any{"manual": "run make release"} }},
		"check deploy with timeout": {project: func(p map[string]any) {
			repoOf(p)["deploy"] = map[string]any{"check": "true", "timeout_minutes": 15}
		}},
		"no deploy": {project: func(p map[string]any) { delete(repoOf(p), "deploy") }, wantErr: "key repos[0].deploy is missing", wantFile: "blog.json"},
		"manual and job": {project: func(p map[string]any) {
			repoOf(p)["deploy"] = map[string]any{"manual": "run make release", "job": "deploy"}
		}, wantErr: "key repos[0].deploy needs manual alone", wantFile: "blog.json"},
		"empty deploy": {project: func(p map[string]any) { repoOf(p)["deploy"] = map[string]any{} }, wantErr: "key repos[0].deploy needs manual alone", wantFile: "blog.json"},
		"zero deploy timeout": {project: func(p map[string]any) {
			repoOf(p)["deploy"] = map[string]any{"job": "deploy", "timeout_minutes": 0}
		}, wantErr: "key repos[0].deploy.timeout_minutes must be a whole number above zero", wantFile: "blog.json"},
		"number deploy job": {project: func(p map[string]any) { repoOf(p)["deploy"] = map[string]any{"job": 5} }, wantErr: "key repos[0].deploy.job must be a string", wantFile: "blog.json"},
		"other team no project": {shop: func() map[string]any {
			p := validProject("shop")
			p["linear_team"] = "SHP"
			delete(p, "linear_project")
			return p
		}()},
		"missing company":    {settings: func(s map[string]any) { delete(s, "company") }, wantErr: "key company is missing", wantFile: "settings.json"},
		"missing agterm":     {settings: func(s map[string]any) { delete(s, "agterm_workspace") }, wantErr: "key agterm_workspace is missing", wantFile: "settings.json"},
		"missing slack":      {settings: func(s map[string]any) { delete(s, "slack") }, wantErr: "key slack.workspace is missing", wantFile: "settings.json"},
		"missing linear":     {settings: func(s map[string]any) { delete(linearOf(s), "workspace") }, wantErr: "key linear.workspace is missing", wantFile: "settings.json"},
		"missing label":      {settings: func(s map[string]any) { delete(linearOf(s), "default_label") }, wantErr: "key linear.default_label is missing", wantFile: "settings.json"},
		"missing phase":      {settings: func(s map[string]any) { delete(statusesOf(s), "qa") }, wantErr: "key linear.statuses.qa is missing", wantFile: "settings.json"},
		"empty phase":        {settings: func(s map[string]any) { statusesOf(s)["qa"] = "" }, wantErr: "key linear.statuses.qa is missing", wantFile: "settings.json"},
		"unknown phase":      {settings: func(s map[string]any) { statusesOf(s)["waiting"] = "Waiting" }, wantErr: "key linear.statuses.waiting is not a phase", wantFile: "settings.json"},
		"products listed":    {settings: func(s map[string]any) { s["products"] = []any{} }, wantErr: "key products", wantFile: "settings.json"},
		"no channel":         {project: func(p map[string]any) { delete(p, "slack_channel") }, wantErr: "key slack_channel is missing", wantFile: "blog.json"},
		"no team":            {project: func(p map[string]any) { delete(p, "linear_team") }, wantErr: "key linear_team is missing", wantFile: "blog.json"},
		"no repos":           {project: func(p map[string]any) { delete(p, "repos") }, wantErr: "key repos has no repo", wantFile: "blog.json"},
		"empty repos":        {project: func(p map[string]any) { p["repos"] = []any{} }, wantErr: "key repos has no repo", wantFile: "blog.json"},
		"repo no name":       {project: func(p map[string]any) { delete(repoOf(p), "name") }, wantErr: "key repos[0].name is missing", wantFile: "blog.json"},
		"repo no branch":     {project: func(p map[string]any) { delete(repoOf(p), "default_branch") }, wantErr: "key repos[0].default_branch is missing", wantFile: "blog.json"},
		"wrong type":         {project: func(p map[string]any) { p["repos"] = projectSecret }, wantErr: "key repos must be a list", wantFile: "blog.json"},
		"mixed-case channel": {project: func(p map[string]any) { p["Slack_Channel"] = p["slack_channel"]; delete(p, "slack_channel") }, wantErr: "key slack_channel is missing", wantFile: "blog.json"},
		"number project":     {project: func(p map[string]any) { p["linear_project"] = 5 }, wantErr: "key linear_project must be a string", wantFile: "blog.json", stricter: true},
		"number status":      {settings: func(s map[string]any) { statusesOf(s)["qa"] = 3 }, wantErr: "key linear.statuses.qa must be a string", wantFile: "settings.json", stricter: true},
		"object override": {project: func(p map[string]any) {
			repoOf(p)["profile_overrides"] = map[string]any{"test": map[string]any{"k": projectSecret}}
		}, wantErr: "key repos[0].profile_overrides.test must be a string, number or boolean", wantFile: "blog.json", stricter: true},
		"null override":      {project: func(p map[string]any) { repoOf(p)["profile_overrides"] = map[string]any{"test": nil} }, wantErr: "key repos[0].profile_overrides.test must be a string, number or boolean", wantFile: "blog.json", stricter: true},
		"shared, no project": {shop: noProject, wantErr: "key linear_project is missing in", wantFile: "shop.json"},
		"same channel": {shop: func() map[string]any {
			p := validProject("blog")
			p["linear_team"] = "SHP"
			return p
		}(), wantErr: "slack_channel blog is in both", wantFile: "shop.json"},
	}
}

func (tc registryCase) write(t *testing.T) string {
	t.Helper()
	settings := validRegistrySettings()
	if tc.settings != nil {
		tc.settings(settings)
	}
	project := validProject("blog")
	project["api_token"] = projectSecret
	if tc.project != nil {
		tc.project(project)
	}
	projects := map[string]map[string]any{"blog": project}
	if tc.shop != nil {
		projects["shop"] = tc.shop
	}
	return writeRegistry(t, settings, projects)
}

func TestLoadRegistry(t *testing.T) {
	for name, tc := range registryCases() {
		t.Run(name, func(t *testing.T) {
			folder := tc.write(t)

			reg, err := config.LoadRegistry(folder)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if reg.Linear.Statuses["pr"] != "In Review" || len(reg.Projects) == 0 {
					t.Fatalf("wrong data: %+v", reg)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), tc.wantFile) {
				t.Fatalf("error = %v, want it to contain %q and %q", err, tc.wantErr, tc.wantFile)
			}
			if strings.Contains(err.Error(), projectSecret) {
				t.Fatalf("error leaks a value: %v", err)
			}
		})
	}
}

func TestLoadRegistry_ProjectValues(t *testing.T) {
	blog := validProject("blog")
	blog["repos"] = []any{
		map[string]any{"name": "blog-web", "default_branch": "develop", "profile_overrides": map[string]any{"diff_coverage_min": 70, "mutation_min": "60", "x": true},
			"deploy": map[string]any{"manual": "run make release"}},
		map[string]any{"name": "blog-api", "default_branch": "main", "deploy": map[string]any{"manual": "run make release"}},
	}
	shop := validProject("shop")
	shop["linear_team"] = "SHP"
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{"blog": blog, "shop": shop})

	reg, err := config.LoadRegistry(folder)

	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Projects) != 2 {
		t.Fatalf("projects = %+v", reg.Projects)
	}
	got, ok := reg.Project("blog")
	if !ok || got.LinearProject != "blog" || len(got.Repos) != 2 {
		t.Fatalf("blog = %+v", got)
	}
	if r := got.Repos[0]; r.Name != "blog-web" || r.DefaultBranch != "develop" || r.ProfileOverrides["diff_coverage_min"] != "70" ||
		r.ProfileOverrides["mutation_min"] != "60" || r.ProfileOverrides["x"] != "true" {
		t.Fatalf("repo 0 = %+v", r)
	}
	if got.LinearTeam != "ABC" {
		t.Fatalf("blog team = %q, want ABC", got.LinearTeam)
	}
	if shop, _ := reg.Project("shop"); shop.LinearTeam != "SHP" {
		t.Fatalf("shop team = %q, want its own team SHP", shop.LinearTeam)
	}
	if _, ok := reg.Project("nope"); ok {
		t.Fatal("unknown project found")
	}
}

func TestLoadRegistry_UnknownProjectKeysAreIgnoredAndNeverKept(t *testing.T) {
	blog := validProject("blog")
	blog["api_token"] = projectSecret
	blog["deploy"] = map[string]any{"password": projectSecret}
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{"blog": blog})

	reg, err := config.LoadRegistry(folder)

	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%+v", reg), projectSecret) {
		t.Fatal("the registry keeps an unknown value")
	}
}

func TestLoadRegistry_DuplicateChannelNamesBothFiles(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{
		"blog": validProject("blog"),
		"blog2": func() map[string]any {
			p := validProject("blog")
			p["linear_team"] = "SHP"
			return p
		}(),
	})

	_, err := config.LoadRegistry(folder)

	if err == nil || !strings.Contains(err.Error(), "blog.json") || !strings.Contains(err.Error(), "blog2.json") ||
		!strings.Contains(err.Error(), "slack_channel") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRegistry_NoProjectFiles(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), nil)
	writeFile(t, filepath.Join(folder, ".workspace", "projects", "notes.txt"), "x", 0o600)

	_, err := config.LoadRegistry(folder)

	if err == nil || !strings.Contains(err.Error(), "no project file") || !strings.Contains(err.Error(), "projects") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRegistry_NoProjectsFolder(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), nil)
	if err := os.Remove(filepath.Join(folder, ".workspace", "projects")); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadRegistry(folder)

	if err == nil || !strings.Contains(err.Error(), "no project file") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRegistry_MissingSettings(t *testing.T) {
	_, err := config.LoadRegistry(t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "settings.json is missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRegistry_InvalidJSONHidesValue(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{"blog": validProject("blog")})
	writeFile(t, filepath.Join(folder, ".workspace", "projects", "blog.json"), `{"slack_channel": "blog", `+projectSecret+`}`, 0o600)

	_, err := config.LoadRegistry(folder)

	if err == nil || !strings.Contains(err.Error(), "blog.json is not valid JSON") || strings.Contains(err.Error(), "character") {
		t.Fatalf("error = %v", err)
	}
}

func linearOf(s map[string]any) map[string]any {
	return s["linear"].(map[string]any)
}

func statusesOf(s map[string]any) map[string]any {
	return linearOf(s)["statuses"].(map[string]any)
}

func repoOf(p map[string]any) map[string]any {
	return p["repos"].([]any)[0].(map[string]any)
}

func TestLoadRegistry_ReadsDeploy(t *testing.T) {
	blog := validProject("blog")
	blog["repos"] = []any{
		map[string]any{"name": "blog-api", "default_branch": "main",
			"deploy": map[string]any{"job": "deploy", "check": "ssh host 'cat version' | grep -q {short_sha}", "timeout_minutes": 15}},
		map[string]any{"name": "blog-web", "default_branch": "main", "deploy": map[string]any{"check": "true"}},
		map[string]any{"name": "blog-docs", "default_branch": "main", "deploy": map[string]any{"manual": "run make release"}},
	}
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{"blog": blog})

	reg, err := config.LoadRegistry(folder)

	if err != nil {
		t.Fatal(err)
	}
	got, _ := reg.Project("blog")
	want := map[string]config.RepoDeploy{
		"blog-api":  {Job: "deploy", Check: "ssh host 'cat version' | grep -q {short_sha}", Timeout: 15 * time.Minute},
		"blog-web":  {Check: "true", Timeout: time.Hour},
		"blog-docs": {Manual: "run make release", Timeout: time.Hour},
	}
	for name, deploy := range want {
		if repo, _ := got.Repo(name); repo.Deploy != deploy {
			t.Fatalf("%s deploy = %+v, want %+v", name, repo.Deploy, deploy)
		}
	}
}

func TestLoadRegistry_MissingDeployNamesTheRepoTheKeyAndTheFile(t *testing.T) {
	blog := validProject("blog")
	delete(repoOf(blog), "deploy")
	folder := writeRegistry(t, validRegistrySettings(), map[string]map[string]any{"blog": blog})

	_, err := config.LoadRegistry(folder)

	if err == nil || !strings.Contains(err.Error(), "repo blog-api") || !strings.Contains(err.Error(), "repos[0].deploy") ||
		!strings.Contains(err.Error(), "blog.json") {
		t.Fatalf("error = %v", err)
	}
}
