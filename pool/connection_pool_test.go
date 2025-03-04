package pool

import (
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockConnectionDisposer implements ConnectionDisposer for testing
type MockConnectionDisposer struct {
	mock.Mock
}

func (m *MockConnectionDisposer) Dispose(obj *SQLiteConnection) error {
	args := m.Called(obj)
	return args.Error(0)
}

// createTestDB creates a temporary SQLite database file for testing
func createTestDB(t *testing.T) (string, func()) {
	tempDir, err := ioutil.TempDir("", "harmonylite-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")

	// Create a minimal database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create test table: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return dbPath, cleanup
}

func TestSQLiteConnection_Init(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		conn := &SQLiteConnection{state: 0}
		mockDisposer := new(MockConnectionDisposer)

		// Test initialization
		err := conn.init(dbPath, mockDisposer)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, conn.db)
		assert.NotNil(t, conn.raw)
		assert.NotNil(t, conn.gSQL)
		assert.Equal(t, mockDisposer, conn.disposer)
		assert.Equal(t, int32(1), conn.state)
	})

	t.Run("already initialized", func(t *testing.T) {
		conn := &SQLiteConnection{state: 1}
		mockDisposer := new(MockConnectionDisposer)

		// Init should return immediately without changing state
		err := conn.init("non-existent.db", mockDisposer)

		assert.NoError(t, err)
		assert.Nil(t, conn.db)
		assert.Nil(t, conn.raw)
		assert.Nil(t, conn.gSQL)
		assert.Nil(t, conn.disposer)
		assert.Equal(t, int32(1), conn.state)
	})

	t.Run("initialization error", func(t *testing.T) {
		conn := &SQLiteConnection{state: 0}
		mockDisposer := new(MockConnectionDisposer)

		// Test initialization with invalid path
		err := conn.init("/invalid/path/that/does/not/exist.db", mockDisposer)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, conn.db)
		assert.Nil(t, conn.raw)
		assert.Nil(t, conn.gSQL)
		assert.Nil(t, conn.disposer)
		assert.Equal(t, int32(0), conn.state) // State should be reset
	})
}

func TestSQLiteConnection_Reset(t *testing.T) {
	t.Run("successful reset", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		// Create and initialize a connection
		conn := &SQLiteConnection{state: 0}
		mockDisposer := new(MockConnectionDisposer)
		err := conn.init(dbPath, mockDisposer)
		assert.NoError(t, err)

		// Reset connection
		conn.reset()

		// Assertions
		assert.Nil(t, conn.db)
		assert.Nil(t, conn.raw)
		assert.Nil(t, conn.gSQL)
		assert.Nil(t, conn.disposer)
		assert.Equal(t, int32(0), conn.state)
	})

	t.Run("already reset", func(t *testing.T) {
		conn := &SQLiteConnection{state: 0}

		// Reset should return immediately without changing state
		conn.reset()

		assert.Equal(t, int32(0), conn.state)
	})
}

func TestNewSQLitePool(t *testing.T) {
	t.Run("lazy initialization", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		// Test creating a pool with lazy initialization
		pool, err := NewSQLitePool(dbPath, 5, true)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, pool)
		assert.Equal(t, dbPath, pool.dns)
		assert.Equal(t, 5, cap(pool.connections))
		assert.Equal(t, 5, len(pool.connections))

		// Check that connections have state = 0 (not initialized)
		conn := <-pool.connections
		assert.Equal(t, int32(0), conn.state)
		pool.connections <- conn // Return it to the pool
	})

	t.Run("eager initialization", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		// Test creating a pool with eager initialization
		pool, err := NewSQLitePool(dbPath, 3, false)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, pool)
		assert.Equal(t, 3, cap(pool.connections))
		assert.Equal(t, 3, len(pool.connections))

		// Check that connections have state = 1 (initialized)
		conn := <-pool.connections
		assert.Equal(t, int32(1), conn.state)
		assert.NotNil(t, conn.db)
		pool.connections <- conn // Return it to the pool
	})

	t.Run("initialization error", func(t *testing.T) {
		// Test creating a pool with eager initialization that fails
		pool, err := NewSQLitePool("/invalid/path/to/db.db", 3, false)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, pool)
	})
}

func TestSQLitePool_Borrow(t *testing.T) {
	t.Run("successful borrow", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		// Create a pool with one connection
		pool, err := NewSQLitePool(dbPath, 1, true)
		assert.NoError(t, err)

		// Borrow the connection
		conn, err := pool.Borrow()

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, conn)
		assert.NotNil(t, conn.db)
		assert.NotNil(t, conn.raw)
		assert.Equal(t, pool, conn.disposer)
		assert.Equal(t, int32(1), conn.state)

		// Pool should be empty now
		assert.Equal(t, 0, len(pool.connections))

		// Return the connection to cleanup
		err = conn.Return()
		assert.NoError(t, err)
	})

	t.Run("initialization error", func(t *testing.T) {
		// Create a pool with one connection
		pool, err := NewSQLitePool("/invalid/path/to/db.db", 1, true)
		assert.NoError(t, err)

		// Borrow the connection
		conn, err := pool.Borrow()

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, conn)

		// Pool should have a new empty connection
		assert.Equal(t, 1, len(pool.connections))
	})
}

func TestSQLitePool_Dispose(t *testing.T) {
	t.Run("dispose to correct pool", func(t *testing.T) {
		dbPath, cleanup := createTestDB(t)
		defer cleanup()

		// Create a pool with one connection
		pool, err := NewSQLitePool(dbPath, 1, true)
		assert.NoError(t, err)

		// Borrow a connection
		conn, err := pool.Borrow()
		assert.NoError(t, err)

		// Test disposing
		err = pool.Dispose(conn)

		// Assertions
		assert.NoError(t, err)
		assert.Equal(t, 1, len(pool.connections))
	})

	t.Run("dispose to wrong pool", func(t *testing.T) {
		dbPath1, cleanup1 := createTestDB(t)
		defer cleanup1()

		dbPath2, cleanup2 := createTestDB(t)
		defer cleanup2()

		// Create two pools
		pool1, err := NewSQLitePool(dbPath1, 1, true)
		assert.NoError(t, err)

		pool2, err := NewSQLitePool(dbPath2, 1, true)
		assert.NoError(t, err)

		// Borrow from pool1
		conn, err := pool1.Borrow()
		assert.NoError(t, err)

		// Try to dispose to pool2
		err = pool2.Dispose(conn)

		// Assertions
		assert.Error(t, err)
		assert.Equal(t, ErrWrongPool, err)
		assert.Equal(t, 1, len(pool2.connections)) // pool2 should still have its original connection

		// Return to correct pool to clean up
		err = pool1.Dispose(conn)
		assert.NoError(t, err)
	})
}

func TestSQLiteConnection_Return(t *testing.T) {
	dbPath, cleanup := createTestDB(t)
	defer cleanup()

	// Create a pool with one connection
	pool, err := NewSQLitePool(dbPath, 1, true)
	assert.NoError(t, err)

	// Borrow the connection
	conn, err := pool.Borrow()
	assert.NoError(t, err)

	// Test returning the connection
	err = conn.Return()

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pool.connections))
}

func TestSQLiteConnection_Accessors(t *testing.T) {
	dbPath, cleanup := createTestDB(t)
	defer cleanup()

	// Create and initialize a connection
	conn := &SQLiteConnection{state: 0}
	mockDisposer := new(MockConnectionDisposer)
	err := conn.init(dbPath, mockDisposer)
	assert.NoError(t, err)

	// Test accessor methods
	assert.NotNil(t, conn.SQL())
	assert.NotNil(t, conn.Raw())
	assert.NotNil(t, conn.DB())

	// Clean up
	conn.reset()
}

func TestSQLitePool_Concurrency(t *testing.T) {
	// Skip for quick tests as this may be slightly slower
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	dbPath, cleanup := createTestDB(t)
	defer cleanup()

	// Create a pool with 5 connections
	poolSize := 5
	pool, err := NewSQLitePool(dbPath, poolSize, false)
	assert.NoError(t, err)

	// Test with 10 goroutines (more than pool size) borrowing and returning
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			// Each goroutine borrows and returns 3 times
			for j := 0; j < 3; j++ {
				conn, err := pool.Borrow()
				if err != nil {
					t.Errorf("Failed to borrow connection: %v", err)
					return
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)

				err = conn.Return()
				if err != nil {
					t.Errorf("Failed to return connection: %v", err)
					return
				}
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// After all operations, the pool should have all connections back
	assert.Equal(t, poolSize, len(pool.connections))
}
