package db

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// migrationFileRE matches the names golang-migrate reads: NNN_name.up.sql
// and NNN_name.down.sql.
var migrationFileRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

// migrationPair holds the files that share one version number.
type migrationPair struct {
	ups   []string // file names, e.g. 060_peace_treaties.up.sql
	downs []string
}

// migrationProblems reports everything wrong with a set of migration file
// names: a version claimed by two files, a version without its partner,
// a file name that does not parse, a gap in the run from 001. It returns
// nothing for a sound set.
func migrationProblems(names []string) []string {
	var problems []string
	byVersion := map[int]*migrationPair{}

	for _, name := range names {
		m := migrationFileRE.FindStringSubmatch(name)
		if m == nil {
			problems = append(problems, fmt.Sprintf(
				"%s is not NNN_name.up.sql or NNN_name.down.sql", name))
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: bad version: %v", name, err))
			continue
		}
		p := byVersion[v]
		if p == nil {
			p = &migrationPair{}
			byVersion[v] = p
		}
		if m[3] == "up" {
			p.ups = append(p.ups, name)
		} else {
			p.downs = append(p.downs, name)
		}
	}

	versions := make([]int, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	for _, v := range versions {
		p := byVersion[v]
		if len(p.ups) > 1 {
			problems = append(problems, clashProblem(v, p.ups, "up"))
		}
		if len(p.downs) > 1 {
			problems = append(problems, clashProblem(v, p.downs, "down"))
		}
		if len(p.ups) == 0 {
			problems = append(problems, fmt.Sprintf(
				"version %03d has no .up.sql (%s)", v, strings.Join(p.downs, ", ")))
		}
		if len(p.downs) == 0 {
			problems = append(problems, fmt.Sprintf(
				"version %03d has no .down.sql (%s)", v, strings.Join(p.ups, ", ")))
		}
		if len(p.ups) == 1 && len(p.downs) == 1 &&
			strings.TrimSuffix(p.ups[0], ".up.sql") != strings.TrimSuffix(p.downs[0], ".down.sql") {
			problems = append(problems, fmt.Sprintf(
				"version %03d: %s and %s have different names, so they are not one migration",
				v, p.ups[0], p.downs[0]))
		}
	}

	if _, ok := byVersion[0]; ok {
		problems = append(problems, "version 000 is not allowed: versions start at 001")
	}
	if len(versions) > 0 {
		top := versions[len(versions)-1]
		for v := 1; v <= top; v++ {
			if _, ok := byVersion[v]; !ok {
				problems = append(problems, fmt.Sprintf(
					"version %03d is missing: versions must run 001, 002, ... with no gaps", v))
			}
		}
	}
	return problems
}

// clashProblem is the failure message for two files sharing a version. It
// says which version clashes and what the later PR must do, because that is
// the fix a reader of a red CI run needs.
func clashProblem(v int, files []string, direction string) string {
	return fmt.Sprintf(
		"version %03d clashes: %s are both .%s.sql files for version %03d. "+
			"Two migrations can't share a version, and golang-migrate applies them in version order. "+
			"The PR that merged second must renumber its migration (and its .down.sql) to the next free version, "+
			"rebase onto main, and merge after the PR it clashes with. Never renumber a migration already on main.",
		v, strings.Join(files, " and "), direction, v)
}

// TestMigrationFilesAreUniqueAndGapFree checks the migrations the binary
// embeds. Two PRs that each add a 060 both pass CI alone; this is what makes
// the second one fail once the first has merged.
func TestMigrationFilesAreUniqueAndGapFree(t *testing.T) {
	entries, err := fs.ReadDir(migrations, "migrations")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	require.NotEmpty(t, names, "no embedded migrations found")

	for _, p := range migrationProblems(names) {
		t.Error(p)
	}
}

// TestMigrationProblems proves the checker itself bites on each way a
// migration set can go wrong, so the real-directory test above is not green
// by accident.
func TestMigrationProblems(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string // each must appear in some problem; nil means none
	}{
		{
			name:  "sound run",
			files: []string{"001_a.up.sql", "001_a.down.sql", "002_b.up.sql", "002_b.down.sql"},
		},
		{
			name:  "two up files share a version",
			files: []string{"001_a.up.sql", "001_a.down.sql", "001_b.up.sql", "001_b.down.sql"},
			want:  []string{"version 001 clashes", "001_a.up.sql and 001_b.up.sql", "renumber"},
		},
		{
			name:  "two down files share a version",
			files: []string{"001_a.up.sql", "001_a.down.sql", "001_b.down.sql", "001_b.up.sql"},
			want:  []string{"version 001 clashes", "001_a.down.sql and 001_b.down.sql"},
		},
		{
			name:  "gap in the run",
			files: []string{"001_a.up.sql", "001_a.down.sql", "003_c.up.sql", "003_c.down.sql"},
			want:  []string{"version 002 is missing"},
		},
		{
			name:  "run does not start at 001",
			files: []string{"002_b.up.sql", "002_b.down.sql"},
			want:  []string{"version 001 is missing"},
		},
		{
			name:  "version 000",
			files: []string{"000_z.up.sql", "000_z.down.sql", "001_a.up.sql", "001_a.down.sql"},
			want:  []string{"version 000 is not allowed"},
		},
		{
			name:  "up without down",
			files: []string{"001_a.up.sql"},
			want:  []string{"version 001 has no .down.sql"},
		},
		{
			name:  "down without up",
			files: []string{"001_a.up.sql", "001_a.down.sql", "002_b.down.sql"},
			want:  []string{"version 002 has no .up.sql"},
		},
		{
			name:  "up and down names differ",
			files: []string{"001_a.up.sql", "001_b.down.sql"},
			want:  []string{"different names"},
		},
		{
			name:  "file that is not a migration",
			files: []string{"001_a.up.sql", "001_a.down.sql", "notes.txt"},
			want:  []string{"notes.txt is not NNN_name.up.sql"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := migrationProblems(tc.files)
			if len(tc.want) == 0 {
				require.Empty(t, got)
				return
			}
			require.NotEmpty(t, got)
			for _, w := range tc.want {
				found := false
				for _, p := range got {
					if strings.Contains(p, w) {
						found = true
						break
					}
				}
				require.Truef(t, found, "no problem mentions %q; got %q", w, got)
			}
		})
	}
}
