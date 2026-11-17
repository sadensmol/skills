package engine_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/engine"
)

func TestChangedFiles_ComparesContentHashes(t *testing.T) {
	before := map[string]string{"a.go": "h1", "b.go": "h2", "gone.go": "h3"}
	after := map[string]string{"a.go": "h1", "b.go": "h9", "new.go": "h4"}

	require.ElementsMatch(t, []string{"b.go", "new.go", "gone.go"}, engine.ChangedFiles(before, after))
}

func TestOutsideZones_ListsFilesOutsideEveryZone(t *testing.T) {
	zones := []string{"src/a/**", "docs/readme.md"}
	changed := []string{"src/a/x.go", "src/a/deep/y.go", "docs/readme.md", "src/b/z.go", "docs/other.md", "src/ax.go"}

	require.Equal(t, []string{"src/b/z.go", "docs/other.md", "src/ax.go"}, engine.OutsideZones(changed, zones))
}

func TestOutsideZones_StarZoneAllowsAll(t *testing.T) {
	require.Empty(t, engine.OutsideZones([]string{"a/b.go"}, []string{"*"}))
}
