package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqlite"
)

// SchemaManager wraps Atlas's SQLite driver for schema operations
type SchemaManager struct {
	driver migrate.Driver
	db     *sql.DB
}

// NewSchemaManager creates a new SchemaManager using the provided database connection
func NewSchemaManager(db *sql.DB) (*SchemaManager, error) {
	// Open Atlas driver on existing connection
	driver, err := sqlite.Open(db)
	if err != nil {
		return nil, fmt.Errorf("opening atlas driver: %w", err)
	}
	return &SchemaManager{driver: driver, db: db}, nil
}

// InspectTables returns Atlas schema.Table objects for the specified tables
func (sm *SchemaManager) InspectTables(ctx context.Context, tables []string) ([]*schema.Table, error) {
	// Inspect the schema realm (all tables)
	realm, err := sm.driver.InspectRealm(ctx, &schema.InspectRealmOption{
		Schemas: []string{"main"},
	})
	if err != nil {
		return nil, fmt.Errorf("inspecting realm: %w", err)
	}

	if len(realm.Schemas) == 0 {
		return nil, nil
	}

	// Filter to requested tables
	tableSet := make(map[string]bool)
	for _, t := range tables {
		tableSet[t] = true
	}

	var result []*schema.Table
	for _, t := range realm.Schemas[0].Tables {
		if tableSet[t.Name] {
			result = append(result, t)
		}
	}
	return result, nil
}

// ComputeSchemaHash computes a deterministic SHA-256 hash of the specified tables
func (sm *SchemaManager) ComputeSchemaHash(ctx context.Context, tables []string) (string, error) {
	inspected, err := sm.InspectTables(ctx, tables)
	if err != nil {
		return "", err
	}

	// Sort tables by name for determinism
	sort.Slice(inspected, func(i, j int) bool {
		return inspected[i].Name < inspected[j].Name
	})

	h := sha256.New()
	for _, table := range inspected {
		if err := hashTable(h, table); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashTable writes a deterministic representation of a table to the hasher
func hashTable(h io.Writer, table *schema.Table) error {
	// Sort columns by name for determinism
	cols := make([]*schema.Column, len(table.Columns))
	copy(cols, table.Columns)
	sort.Slice(cols, func(i, j int) bool {
		return cols[i].Name < cols[j].Name
	})

	// Write table name
	h.Write([]byte(table.Name))

	// Write each column: |name:type:notnull:pk
	for _, col := range cols {
		isPK := false
		if table.PrimaryKey != nil {
			for _, pkCol := range table.PrimaryKey.Parts {
				if pkCol.C != nil && pkCol.C.Name == col.Name {
					isPK = true
					break
				}
			}
		}

		// Normalize type string using Atlas's type representation
		typeStr := col.Type.Raw
		if typeStr == "" && col.Type.Type != nil {
			typeStr = fmt.Sprintf("%T", col.Type.Type)
		}

		h.Write([]byte(fmt.Sprintf("|%s:%s:%t:%t",
			col.Name, typeStr, !col.Type.Null, isPK)))
	}
	h.Write([]byte("\n"))
	return nil
}

// Close closes the Atlas driver connection (noop as Atlas driver doesn't expose Close)
func (sm *SchemaManager) Close() error {
	// Atlas migrate.Driver interface doesn't have Close method
	// The underlying DB connection is managed externally
	return nil
}
