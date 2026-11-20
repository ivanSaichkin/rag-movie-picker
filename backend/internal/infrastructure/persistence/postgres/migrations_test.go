package postgres

import (
	"strings"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	version, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("LatestVersion() = %d, want 1", version)
	}
}

func TestMigrationURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "postgres scheme",
			input: "postgres://u:p@localhost:5432/app?sslmode=disable",
			want:  "pgx5://u:p@localhost:5432/app?sslmode=disable",
		},
		{
			name:  "postgresql scheme",
			input: "postgresql://u:p@localhost:5432/app",
			want:  "pgx5://u:p@localhost:5432/app",
		},
		{
			name:  "already migrated scheme",
			input: "pgx5://u:p@localhost:5432/app",
			want:  "pgx5://u:p@localhost:5432/app",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := migrationURL(testCase.input)
			if err != nil {
				t.Fatalf("migrationURL(%q) error = %v", testCase.input, err)
			}
			if got != testCase.want {
				t.Fatalf("migrationURL(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestMigrationURLRejectsUnsupportedScheme(t *testing.T) {
	for _, dsn := range []string{"mysql://u:p@localhost:3306/app", "sqlite://file.db", "://bad"} {
		_, err := migrationURL(dsn)
		if err == nil {
			t.Fatalf("migrationURL(%q) must fail", dsn)
		}
		if !strings.Contains(err.Error(), "DATABASE_URL") {
			t.Fatalf("error %q must mention DATABASE_URL", err)
		}
	}
}

func TestVersionFailsOnUnsupportedScheme(t *testing.T) {
	_, _, err := Version("mysql://u:p@localhost:3306/app")
	if err == nil {
		t.Fatal("Version() must fail for an unsupported scheme")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error %q must mention DATABASE_URL", err)
	}
}

func TestDownRejectsNonPositiveSteps(t *testing.T) {
	for _, steps := range []int{0, -1} {
		err := Down("postgres://u:p@localhost:5432/app", steps)
		if err == nil {
			t.Fatalf("Down(%d) must fail", steps)
		}
	}
}
