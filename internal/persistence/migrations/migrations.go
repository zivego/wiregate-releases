package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed sql/*.sql
var migrationFS embed.FS

type Migration struct {
	Name string
	SQL  string
}

func LoadAll() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "sql")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	out := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := "sql/" + entry.Name()
		body, readErr := migrationFS.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), readErr)
		}
		out = append(out, Migration{Name: entry.Name(), SQL: string(body)})
	}

	return out, nil
}
