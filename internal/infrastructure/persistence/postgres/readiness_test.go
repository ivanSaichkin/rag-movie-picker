package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(context.Context) error { return f.err }

type fakeSchema struct {
	version uint
	dirty   bool
	err     error
}

func (f fakeSchema) SchemaVersion(context.Context) (uint, bool, error) {
	return f.version, f.dirty, f.err
}

func TestReadinessPassesWhenSchemaIsCurrent(t *testing.T) {
	check := Readiness{
		db:     fakePinger{},
		schema: fakeSchema{version: 3},
		latest: 3,
	}

	if err := check.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if check.Name() != "postgres" {
		t.Fatalf("Name() = %q, want postgres", check.Name())
	}
}

func TestReadinessReportsPingFailure(t *testing.T) {
	check := Readiness{
		db:     fakePinger{err: errors.New("connection refused")},
		schema: fakeSchema{version: 1},
		latest: 1,
	}

	err := check.Check(context.Background())
	if err == nil {
		t.Fatal("Check() must fail when the database is unreachable")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error %q must keep the cause", err)
	}
}

func TestReadinessRejectsDirtySchema(t *testing.T) {
	check := Readiness{
		db:     fakePinger{},
		schema: fakeSchema{version: 2, dirty: true},
		latest: 2,
	}

	err := check.Check(context.Background())
	if err == nil {
		t.Fatal("Check() must fail on a dirty schema")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("error %q must mention the dirty state", err)
	}
}

func TestReadinessRejectsStaleSchema(t *testing.T) {
	check := Readiness{
		db:     fakePinger{},
		schema: fakeSchema{version: 1},
		latest: 2,
	}

	err := check.Check(context.Background())
	if err == nil {
		t.Fatal("Check() must fail when migrations are not applied")
	}
	if !strings.Contains(err.Error(), "schema version 1, want 2") {
		t.Fatalf("error %q must report both versions", err)
	}
}

func TestReadinessReportsSchemaReadFailure(t *testing.T) {
	check := Readiness{
		db:     fakePinger{},
		schema: fakeSchema{err: errors.New("boom")},
		latest: 1,
	}

	if err := check.Check(context.Background()); err == nil {
		t.Fatal("Check() must fail when the schema version cannot be read")
	}
}

func TestNewReadinessUsesEmbeddedVersion(t *testing.T) {
	check, err := NewReadiness(nil)
	if err != nil {
		t.Fatalf("NewReadiness() error = %v", err)
	}
	if check.latest != 1 {
		t.Fatalf("latest = %d, want 1", check.latest)
	}
}
