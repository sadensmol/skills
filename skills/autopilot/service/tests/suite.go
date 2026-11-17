package tests

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/repository"
)

var StartTime = time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)

type Clock struct {
	now time.Time
}

func (c *Clock) Now() time.Time {
	return c.now
}

func (c *Clock) Set(t time.Time) {
	c.now = t
}

func (c *Clock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

// Suite gives every integration test a clock and a fresh temp database.
type Suite struct {
	suite.Suite
	Ctx    context.Context
	Clock  *Clock
	DBPath string
	DB     *sql.DB
}

func (s *Suite) SetupTest() {
	s.Ctx = context.Background()
	s.Clock = &Clock{now: StartTime}
	s.DBPath = filepath.Join(s.T().TempDir(), repository.DBFileName)
	db, err := repository.Open(s.Ctx, s.DBPath)
	s.Require().NoError(err)
	s.DB = db
}

func (s *Suite) TearDownTest() {
	s.Require().NoError(s.DB.Close())
}

func (s *Suite) CountRows(table string) int {
	var n int
	s.Require().NoError(s.DB.QueryRowContext(s.Ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&n))
	return n
}
