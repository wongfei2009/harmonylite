package db

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaCache_PreviousHash(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create initial schema
	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)

	// Create schema manager
	sm, err := NewSchemaManager(db)
	require.NoError(t, err)

	// Initialize schema cache
	sc := NewSchemaCache()
	ctx := context.Background()
	err = sc.Initialize(ctx, sm, []string{"users"})
	require.NoError(t, err)

	// Get initial hash
	initialHash := sc.GetSchemaHash()
	assert.NotEmpty(t, initialHash)

	// Previous hash should be empty initially
	assert.Empty(t, sc.GetPreviousHash())

	// Modify the schema
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN email TEXT`)
	require.NoError(t, err)

	// Recompute the hash
	newHash, err := sc.Recompute(ctx)
	require.NoError(t, err)

	// New hash should be different
	assert.NotEqual(t, initialHash, newHash)

	// Previous hash should now be the initial hash
	assert.Equal(t, initialHash, sc.GetPreviousHash())

	// Current hash should be the new hash
	assert.Equal(t, newHash, sc.GetSchemaHash())
}

func TestSchemaCache_PreviousHashUnchangedWhenSameSchema(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create initial schema
	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)

	// Create schema manager
	sm, err := NewSchemaManager(db)
	require.NoError(t, err)

	// Initialize schema cache
	sc := NewSchemaCache()
	ctx := context.Background()
	err = sc.Initialize(ctx, sm, []string{"users"})
	require.NoError(t, err)

	initialHash := sc.GetSchemaHash()

	// Recompute without schema change
	newHash, err := sc.Recompute(ctx)
	require.NoError(t, err)

	// Hash should be the same
	assert.Equal(t, initialHash, newHash)

	// Previous hash should still be empty (no change occurred)
	assert.Empty(t, sc.GetPreviousHash())
}

func TestSchemaCache_MultipleSchemaChanges(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create initial schema
	_, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)

	// Create schema manager
	sm, err := NewSchemaManager(db)
	require.NoError(t, err)

	// Initialize schema cache
	sc := NewSchemaCache()
	ctx := context.Background()
	err = sc.Initialize(ctx, sm, []string{"users"})
	require.NoError(t, err)

	hash1 := sc.GetSchemaHash()

	// First schema change
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN email TEXT`)
	require.NoError(t, err)

	hash2, err := sc.Recompute(ctx)
	require.NoError(t, err)
	assert.Equal(t, hash1, sc.GetPreviousHash())

	// Second schema change
	_, err = db.Exec(`ALTER TABLE users ADD COLUMN phone TEXT`)
	require.NoError(t, err)

	hash3, err := sc.Recompute(ctx)
	require.NoError(t, err)

	// Previous hash should now be hash2 (only tracks one version back)
	assert.Equal(t, hash2, sc.GetPreviousHash())
	assert.Equal(t, hash3, sc.GetSchemaHash())

	// Verify all three hashes are different
	assert.NotEqual(t, hash1, hash2)
	assert.NotEqual(t, hash2, hash3)
	assert.NotEqual(t, hash1, hash3)
}
