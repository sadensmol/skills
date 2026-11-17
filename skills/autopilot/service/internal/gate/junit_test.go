package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goSubtestPanic is gotestsum v1.13.0's report of a subtest that dereferences a nil pointer: only the parent's
// output holds the panic.
const goSubtestPanic = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="3" errors="0">
	<testsuite tests="3" failures="3" name="example.test/probe">
		<testcase classname="example.test/probe" name="TestParent/case/deeper">
			<failure message="Failed" type="">=== RUN   TestParent/case/deeper&#xA;--- FAIL: TestParent/case/deeper (0.00s)&#xA;</failure>
		</testcase>
		<testcase classname="example.test/probe" name="TestParent/case">
			<failure message="Failed" type="">=== RUN   TestParent/case&#xA;--- FAIL: TestParent/case (0.00s)&#xA;</failure>
		</testcase>
		<testcase classname="example.test/probe" name="TestParent">
			<failure message="Failed" type="">=== RUN   TestParent&#xA;--- FAIL: TestParent (0.00s)&#xA;panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]&#xA;[signal SIGSEGV: segmentation violation code=0x2 addr=0x0 pc=0x10491ad2c]&#xA;</failure>
		</testcase>
		<testcase classname="example.test/probe" name="TestTable/ok">
			<failure message="Failed" type="">=== RUN   TestTable/ok&#xA;    p_test.go:19: got 2, want 3&#xA;--- FAIL: TestTable/ok (0.00s)&#xA;</failure>
		</testcase>
		<testcase classname="example.test/probe" name="TestTable">
			<failure message="Failed" type="">=== RUN   TestTable&#xA;--- FAIL: TestTable (0.00s)&#xA;</failure>
		</testcase>
	</testsuite>
</testsuites>`

func TestJUnitResults_GoSubtestOfAPanickedParentIsAnError(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.test/probe\n"), 0o644))
	report := filepath.Join(t.TempDir(), "junit.xml")
	require.NoError(t, os.WriteFile(report, []byte(goSubtestPanic), 0o644))
	det, err := DetectProfile(repo, nil)
	require.NoError(t, err)
	cases, err := readJUnit(report)
	require.NoError(t, err)

	results := newJUnitResults(cases, det.Profile, repo)

	for id, want := range map[string]outcome{
		"example.test/probe.TestParent":             outcomeErrored,
		"example.test/probe.TestParent/case":        outcomeErrored,
		"example.test/probe.TestParent/case/deeper": outcomeErrored,
		"example.test/probe.TestTable/ok":           outcomeFailed,
		"example.test/probe.TestTable":              outcomeFailed,
	} {
		got, ok := results.outcomeOf(id)
		assert.True(t, ok, id)
		assert.Equal(t, want, got, id)
	}
}
