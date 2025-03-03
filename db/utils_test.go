package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestEnhancedRows_FetchRow(t *testing.T) {
	// Create temporary file for the test database
	tmpFile, err := os.CreateTemp("", "harmonylite-utils-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Open the database
	db, err := sql.Open("sqlite3", tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test table with multiple column types
	_, err = db.Exec(`
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY,
			text_col TEXT,
			int_col INTEGER,
			real_col REAL,
			null_col TEXT
		);
		
		INSERT INTO test_table (id, text_col, int_col, real_col, null_col)
		VALUES (1, 'test text', 42, 3.14, NULL);
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Execute a query to get rows
	rows, err := db.Query("SELECT * FROM test_table")
	if err != nil {
		t.Fatalf("Failed to query test table: %v", err)
	}

	// Wrap in EnhancedRows
	enhancedRows := &EnhancedRows{rows}
	defer enhancedRows.Finalize()

	// Test fetching the row
	if !enhancedRows.Next() {
		t.Fatalf("Expected at least one row")
	}

	row, err := enhancedRows.fetchRow()
	if err != nil {
		t.Fatalf("Failed to fetch row: %v", err)
	}

	// Verify row contents
	if row["id"] != int64(1) {
		t.Errorf("Expected id=1, got %v (type: %T)", row["id"], row["id"])
	}

	if row["text_col"] != "test text" {
		t.Errorf("Expected text_col='test text', got %v", row["text_col"])
	}

	if row["int_col"] != int64(42) {
		t.Errorf("Expected int_col=42, got %v (type: %T)", row["int_col"], row["int_col"])
	}

	if row["real_col"] != 3.14 {
		t.Errorf("Expected real_col=3.14, got %v", row["real_col"])
	}

	if row["null_col"] != nil {
		t.Errorf("Expected null_col=nil, got %v", row["null_col"])
	}

	// Test that there are no more rows
	if enhancedRows.Next() {
		t.Errorf("Expected only one row")
	}
}

func TestEnhancedStatement_Finalize(t *testing.T) {
	// Create temporary file for the test database
	tmpFile, err := os.CreateTemp("", "harmonylite-utils-stmt-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Open the database
	db, err := sql.Open("sqlite3", tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE test_statement (
			id INTEGER PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Prepare a statement
	stmt, err := db.Prepare("INSERT INTO test_statement (value) VALUES (?)")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}

	// Wrap in EnhancedStatement
	enhancedStmt := &EnhancedStatement{stmt}

	// Execute the statement
	_, err = enhancedStmt.Exec("test value")
	if err != nil {
		t.Fatalf("Failed to execute statement: %v", err)
	}

	// Finalize the statement
	enhancedStmt.Finalize()

	// Verify that the statement is closed by trying to use it again
	_, err = enhancedStmt.Exec("should fail")
	if err == nil {
		t.Errorf("Expected error after finalizing statement, but got none")
	}
}

func TestEnhancedRows_Finalize(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "harmonylite-rows-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE test_rows (
			id INTEGER PRIMARY KEY,
			value TEXT
		);
		
		INSERT INTO test_rows (value) VALUES ('test1'), ('test2'), ('test3');
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Execute a query
	rows, err := db.Query("SELECT * FROM test_rows")
	if err != nil {
		t.Fatalf("Failed to query test table: %v", err)
	}

	// Wrap in EnhancedRows
	enhancedRows := &EnhancedRows{rows}

	// Consume some but not all rows
	if !enhancedRows.Next() {
		t.Fatalf("Expected at least one row")
	}

	// Finalize before consuming all rows
	enhancedRows.Finalize()

	// Verify that rows are closed by trying to use them again
	if enhancedRows.Next() {
		t.Errorf("Expected closed rows, but Next() returned true")
	}

	// This should not panic if rows were properly closed
	enhancedRows.Finalize()
}
