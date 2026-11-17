package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadensmol/autopilot/internal/config"
)

// registryPy is the registry reader of the project skill; it owns the registry schema. The service lives in the
// autopilot skill, so the script is in the same skills tree.
var registryPy = filepath.Join("..", "..", "..", "..", "project", "scripts", "registry.py")

// runRegistryPy runs registry.py on a workspace folder and reports whether it exited 0. It skips the test when
// python3 or the script is not there, as in a copy of the service outside the skills tree.
func runRegistryPy(t *testing.T, folder string, args ...string) (bool, string) {
	t.Helper()
	if _, err := os.Stat(registryPy); err != nil {
		t.Skipf("registry.py is not at %s", registryPy)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH")
	}
	cmd := exec.Command(python, append([]string{"-I", registryPy}, args...)...)
	cmd.Env = append(os.Environ(), "SADENSMOL_DIR="+folder)
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

func TestLoadRegistry_AcceptsWhatRegistryPyAccepts(t *testing.T) {
	for name, tc := range registryCases() {
		t.Run(name, func(t *testing.T) {
			folder := tc.write(t)

			pyOK, out := runRegistryPy(t, folder, "check")
			_, err := config.LoadRegistry(folder)

			if tc.stricter {
				if !pyOK || err == nil {
					t.Fatalf("stricter case: registry.py check ok=%v (%s), LoadRegistry error = %v; want py ok and a Go error", pyOK, out, err)
				}
				return
			}
			if pyOK != (err == nil) {
				t.Fatalf("registry.py check ok=%v (%s), LoadRegistry error = %v", pyOK, out, err)
			}
			if pyOK != (tc.wantErr == "") {
				t.Fatalf("registry.py check ok=%v (%s), the case expects ok=%v", pyOK, out, tc.wantErr == "")
			}
		})
	}
}

func TestLoadRegistry_LoadsWhatRegistryPyAddProductWrites(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), nil)

	ok, out := runRegistryPy(t, folder, "add-product", "--channel", "blog", "--team", "ABC", "--repo", "blog-api:main",
		"--deploy", `blog-api={"job": "deploy"}`)
	if !ok {
		t.Fatalf("add-product: %s", out)
	}
	reg, err := config.LoadRegistry(folder)

	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	p, found := reg.Project("blog")
	if !found || p.LinearTeam != "ABC" || p.LinearProject != "" || len(p.Repos) != 1 || p.Repos[0].DefaultBranch != "main" ||
		p.Repos[0].Deploy.Job != "deploy" {
		t.Fatalf("blog = %+v", p)
	}
}

func TestRegistryPyAddProductRefusesARepoWithoutDeploy(t *testing.T) {
	folder := writeRegistry(t, validRegistrySettings(), nil)

	ok, out := runRegistryPy(t, folder, "add-product", "--channel", "blog", "--team", "ABC", "--repo", "blog-api:main")

	if ok || !strings.Contains(out, "repos[0].deploy is missing") {
		t.Fatalf("add-product without deploy: ok=%v, %s", ok, out)
	}
}
