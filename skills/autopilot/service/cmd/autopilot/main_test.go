package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sadensmol/autopilot/tests"
)

func TestRun_ExitCodes(t *testing.T) {
	tests := map[string]struct {
		args []string
		want int
	}{
		"version":        {args: []string{"version"}, want: exitOK},
		"no args":        {want: exitUsage},
		"unknown verb":   {args: []string{"nope"}, want: exitUsage},
		"serve":          {args: []string{"serve"}, want: exitUsage},
		"supervise":      {args: []string{"supervise"}, want: exitUsage},
		"check-products": {args: []string{"check-products"}, want: exitUsage},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := run(tc.args); got != tc.want {
				t.Fatalf("run(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

const (
	registrySettings = `{"company": "acme", "agterm_workspace": "acme", "slack": {"workspace": "acme"},
  "linear": {"workspace": "acme", "team": "ABC", "default_label": "autopilot",
    "statuses": {"todo": "a", "planning": "b", "implementing": "c", "qa": "d", "review": "e", "pr": "f", "blocked": "g", "done": "h", "canceled": "i"}}}`
	projectToken = "tok-project-secret"
)

// newWorkspace writes a settings folder and a workspace registry with one file per project, and returns the
// workspace folder and the settings folder.
func newWorkspace(t *testing.T, projects map[string]string) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".workspace", "autopilot")
	for _, d := range []string{dir, filepath.Join(root, ".workspace", "projects")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(dir, "config.yaml"), "owner:\n  slack_user_id: U0ABC\nservice:\n  source_dir: /src\n")
	writeTestFile(t, filepath.Join(dir, "secrets.env"), "SLACK_APP_TOKEN=xapp-secret\nSLACK_BOT_TOKEN=xoxb-secret\nLINEAR_API_KEY=lin-secret\n")
	writeTestFile(t, filepath.Join(root, ".workspace", "settings.json"), registrySettings)
	for name, body := range projects {
		writeTestFile(t, filepath.Join(root, ".workspace", "projects", name+".json"), body)
	}
	return root, dir
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// project is a project file; an empty linearProject leaves the key out.
func project(channel, team, linearProject string) string {
	projectKey := ""
	if linearProject != "" {
		projectKey = fmt.Sprintf(`"linear_project": %q, `, linearProject)
	}
	return fmt.Sprintf(`{"slack_channel": %q, "linear_team": %q, %s"api_token": %q, "repos": [{"name": %q, "default_branch": "main", "deploy": {"manual": "run make release"}}]}`,
		channel, team, projectKey, projectToken, channel)
}

func TestCheckConfig(t *testing.T) {
	root, dir := newWorkspace(t, map[string]string{"abc": project("abc", "ABC", "")})

	var out, errOut bytes.Buffer
	if got := checkConfig([]string{"--settings", dir}, &out, &errOut); got != exitOK || strings.TrimSpace(out.String()) != "OK" {
		t.Fatalf("valid: code %d, out %q, err %q", got, out.String(), errOut.String())
	}

	writeTestFile(t, filepath.Join(root, ".workspace", "settings.json"), `{"company": "acme"}`)
	out.Reset()
	errOut.Reset()
	if got := checkConfig([]string{"--settings", dir}, &out, &errOut); got != exitError || !strings.Contains(errOut.String(), "agterm_workspace") {
		t.Fatalf("bad registry: code %d, err %q", got, errOut.String())
	}

	if got := checkConfig([]string{}, &out, &errOut); got != exitUsage {
		t.Fatalf("missing flag: code %d", got)
	}
	if got := checkConfig([]string{"--settings", dir, "--business", "b.yaml"}, &out, &errOut); got != exitUsage {
		t.Fatalf("business flag: code %d", got)
	}
}

func TestCheckConfig_MissingDeployNamesTheRepoTheKeyAndTheFile(t *testing.T) {
	noDeploy := `{"slack_channel": "abc", "linear_team": "ABC", "repos": [{"name": "abc-api", "default_branch": "main"}]}`
	_, dir := newWorkspace(t, map[string]string{"abc": noDeploy})

	var out, errOut bytes.Buffer
	got := checkConfig([]string{"--settings", dir}, &out, &errOut)

	if got != exitError || !strings.Contains(errOut.String(), "repo abc-api") || !strings.Contains(errOut.String(), "repos[0].deploy is missing") ||
		!strings.Contains(errOut.String(), "abc.json") {
		t.Fatalf("code %d, err %q", got, errOut.String())
	}
}

func TestCheckLinear(t *testing.T) {
	_, dir := newWorkspace(t, map[string]string{"abc": project("abc", "DEF", "abc"), "def": project("def", "GHI", "def"), "ops": project("ops", "OPS", "")})
	fake := tests.NewFakeLinear(t)
	fake.On("Viewer", func(map[string]any) tests.LinearReply {
		return tests.LinearReply{Data: map[string]any{"viewer": map[string]any{"id": "u1", "organization": map[string]any{"urlKey": "acme"}}}}
	})
	fake.On("TeamByKey", func(vars map[string]any) tests.LinearReply {
		key := vars["key"].(string)
		return tests.LinearReply{Data: map[string]any{"teams": map[string]any{"nodes": []map[string]any{{"id": "id-" + key, "key": key, "name": key}}}}}
	})
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	fake.On("WorkflowStates", func(map[string]any) tests.LinearReply {
		nodes := []map[string]any{}
		for _, n := range names {
			nodes = append(nodes, map[string]any{"id": n, "name": n, "type": "started"})
		}
		return tests.LinearReply{Data: map[string]any{"workflowStates": map[string]any{"nodes": nodes}}}
	})
	fake.On("LabelByName", func(map[string]any) tests.LinearReply {
		return tests.LinearReply{Data: map[string]any{"issueLabels": map[string]any{"nodes": []map[string]any{{"id": "l1", "name": "autopilot"}}}}}
	})
	projects := map[string]string{"abc": "id-DEF", "def": "id-GHI"}
	fake.On("ProjectByName", func(vars map[string]any) tests.LinearReply {
		nodes := []map[string]any{}
		if team, ok := projects[vars["name"].(string)]; ok {
			nodes = append(nodes, map[string]any{"id": "p1", "name": vars["name"], "teams": map[string]any{"nodes": []map[string]any{{"id": team}}}})
		}
		return tests.LinearReply{Data: map[string]any{"projects": map[string]any{"nodes": nodes}}}
	})
	args := []string{"--settings", dir, "--endpoint", fake.URL()}

	var out, errOut bytes.Buffer
	if got := checkLinear(args, &out, &errOut); got != exitOK || strings.TrimSpace(out.String()) != "OK" {
		t.Fatalf("valid: code %d, out %q, err %q", got, out.String(), errOut.String())
	}
	teams := map[string]bool{}
	for _, r := range fake.Requests() {
		if r.Operation == "TeamByKey" {
			teams[r.Variables["key"].(string)] = true
		}
	}
	if !teams["ABC"] || !teams["DEF"] || !teams["GHI"] || !teams["OPS"] {
		t.Fatalf("teams checked %v, want the settings team ABC and the project teams DEF, GHI and OPS", teams)
	}
	for _, r := range fake.Requests() {
		if r.Operation == "ProjectByName" && r.Variables["name"] == "ops" {
			t.Fatal("check-linear looked up a Linear project for ops, which names none")
		}
	}

	delete(projects, "def")
	out.Reset()
	errOut.Reset()
	if got := checkLinear(args, &out, &errOut); got != exitError || !strings.Contains(errOut.String(), "GHI: def") {
		t.Fatalf("missing project: code %d, err %q", got, errOut.String())
	}

	projects["def"] = "id-GHI"
	names = names[:8]
	out.Reset()
	errOut.Reset()
	if got := checkLinear(args, &out, &errOut); got != exitError || !strings.Contains(errOut.String(), "ABC: i") {
		t.Fatalf("missing status: code %d, err %q", got, errOut.String())
	}
	if got := checkLinear([]string{}, &out, &errOut); got != exitUsage {
		t.Fatalf("missing flag: code %d", got)
	}
}

func TestCheckProducts(t *testing.T) {
	root, dir := newWorkspace(t, map[string]string{
		"blog": project("blog", "ABC", "blog"), "shop": project("shop", "ABC", "shop"), "wiki": project("wiki", "ABC", "wiki"),
		"news": project("news", "NWS", ""),
	})
	for _, repo := range []string{"blog", "news"} {
		if out, err := exec.Command("git", "init", "-q", filepath.Join(root, repo)).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeLinear := tests.NewFakeLinear(t)
	fakeLinear.On("TeamByKey", func(map[string]any) tests.LinearReply {
		return tests.LinearReply{Data: map[string]any{"teams": map[string]any{"nodes": []map[string]any{{"id": "t1", "key": "ABC", "name": "Abc"}}}}}
	})
	fakeLinear.On("ProjectByName", func(vars map[string]any) tests.LinearReply {
		nodes := []map[string]any{}
		if vars["name"] != "shop" {
			nodes = append(nodes, map[string]any{"id": "p1", "name": vars["name"], "teams": map[string]any{"nodes": []map[string]any{{"id": "t1"}}}})
		}
		return tests.LinearReply{Data: map[string]any{"projects": map[string]any{"nodes": nodes}}}
	})
	fakeSlack := tests.NewFakeSlack(t)
	fakeSlack.Channels = []tests.SlackChannel{{ID: "C1", Name: "blog", IsMember: true}, {ID: "C2", Name: "shop"}, {ID: "C4", Name: "news", IsMember: true}}
	args := []string{"--settings", dir, "--endpoint", fakeLinear.URL(), "--slack-api", fakeSlack.Server.URL + "/api/"}

	var out, errOut bytes.Buffer
	got := checkProducts(args, &out, &errOut)

	if got != exitError {
		t.Fatalf("code %d, out %q, err %q", got, out.String(), errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"#blog: channel ok; project blog ok; repo blog ok",
		"#news: channel ok; project: none; repo news ok",
		"#shop: channel missing (the bot is not a member of `#shop`); project shop missing (no Linear project `shop` in team ABC); repo shop missing (no repo folder for `shop`)",
		"#wiki: channel missing (no Slack channel `#wiki`); project wiki ok; repo wiki missing (repo folder for `wiki` is not a git checkout)",
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	for _, r := range fakeLinear.Requests() {
		if r.Operation == "ProjectByName" && r.Variables["name"] == "" {
			t.Fatal("check-products looked up a Linear project for news, which names none")
		}
	}
	for _, secret := range []string{projectToken, "xoxb-secret", "lin-secret", root} {
		if strings.Contains(out.String()+errOut.String(), secret) {
			t.Fatalf("output carries %q: %s %s", secret, out.String(), errOut.String())
		}
	}

	_, okDir := newWorkspace(t, map[string]string{"blog": project("blog", "ABC", "")})
	if out, err := exec.Command("git", "init", "-q", filepath.Join(filepath.Dir(filepath.Dir(okDir)), "blog")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	out.Reset()
	if got := checkProducts([]string{"--settings", okDir, "--endpoint", fakeLinear.URL(), "--slack-api", fakeSlack.Server.URL + "/api/"}, &out, &errOut); got != exitOK {
		t.Fatalf("all good: code %d, out %q, err %q", got, out.String(), errOut.String())
	}
	if got := checkProducts([]string{}, &out, &errOut); got != exitUsage {
		t.Fatalf("missing flag: code %d", got)
	}
}

func TestServe_RefusesMissingSettingsFolder(t *testing.T) {
	var errOut bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nope")

	got := serve(t.Context(), []string{"--agterm-socket", "/tmp/ap-none.sock", "--settings", missing}, &errOut)

	if got != exitError || !strings.Contains(errOut.String(), missing) {
		t.Fatalf("code %d, err %q", got, errOut.String())
	}
}

func TestServe_RefusesMissingAgtermSocket(t *testing.T) {
	t.Setenv("AGTERM_SOCKET", "")
	var errOut bytes.Buffer

	got := serve(t.Context(), []string{"--settings", t.TempDir()}, &errOut)

	if got != exitUsage || !strings.Contains(errOut.String(), "agterm socket") {
		t.Fatalf("code %d, err %q", got, errOut.String())
	}
}

func TestWorkspaceKey(t *testing.T) {
	for in, want := range map[string]string{"https://acme.slack.com/": "acme", "https://Acme.slack.com": "acme", "": "", "::bad": ""} {
		if got := workspaceKey(in); got != want {
			t.Fatalf("workspaceKey(%q) = %q, want %q", in, got, want)
		}
	}
}
