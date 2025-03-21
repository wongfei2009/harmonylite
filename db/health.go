package db

import "github.com/rs/zerolog/log"

// IsConnected checks if the database connection is alive
func (conn *SqliteStreamDB) IsConnected() bool {
	if conn.pool == nil {
		return false
	}

	// Try to borrow a connection from the pool to check if the database is accessible
	sqlConn, err := conn.pool.Borrow()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to borrow connection from pool")
		return false
	}
	defer sqlConn.Return()

	// Execute a simple query to check if the connection is working
	_, err = sqlConn.DB().Exec("SELECT 1")
	if err != nil {
		log.Debug().Err(err).Msg("Database connectivity check failed")
		return false
	}

	return true
}

// AreCDCHooksInstalled checks if the CDC hooks are installed
func (conn *SqliteStreamDB) AreCDCHooksInstalled() bool {
	if conn.pool == nil {
		return false
	}

	sqlConn, err := conn.pool.Borrow()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to borrow connection from pool")
		return false
	}
	defer sqlConn.Return()

	// Check if any triggers exist that match the expected pattern
	// Looking for triggers that might have either "__harmonylite__" or "**harmonylite**" in their names
	var count int
	err = sqlConn.DB().QueryRow("SELECT count(*) FROM sqlite_master WHERE type='trigger' AND (name LIKE ? OR name LIKE ?)", 
		"__harmonylite__%", "**harmonylite**%").Scan(&count)
	
	if err != nil {
		log.Debug().Err(err).Msg("Error checking for CDC triggers")
		return false
	}

	if count > 0 {
		log.Debug().Int("count", count).Msg("Found CDC triggers")
		return true
	}

	// If no triggers found, let's log what tables and triggers exist to help diagnose the issue
	log.Debug().Msg("No CDC triggers found, checking for tables and triggers")
	
	tables := []string{}
	rows, err := sqlConn.DB().Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				tables = append(tables, name)
			}
		}
	}
	log.Debug().Strs("tables", tables).Msg("Tables in database")
	
	triggers := []string{}
	rows, err = sqlConn.DB().Query("SELECT name FROM sqlite_master WHERE type='trigger'")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				triggers = append(triggers, name)
			}
		}
	}
	log.Debug().Strs("triggers", triggers).Msg("Triggers in database")
	
	return false
}

// GetTrackedTablesCount returns the number of tables being tracked
func (conn *SqliteStreamDB) GetTrackedTablesCount() int {
	return len(conn.watchTablesSchema)
}

// DB returns the underlying database for health check purposes
func (conn *SqliteStreamDB) DB() interface{} {
	return conn.pool
}
