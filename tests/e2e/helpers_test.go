package main_test

import (
	"database/sql"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// -- Cluster Management --

// startCluster initializes and starts a 3-node HarmonyLite cluster.
func startCluster() (node1, node2, node3 *exec.Cmd) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Starting cluster setup...\n")

	// Start nodes with NATS health checks
	node1 = startNode("examples/node-1-config.toml", "127.0.0.1:4221", "nats://127.0.0.1:4222/,nats://127.0.0.1:4223/")
	node2 = startNode("examples/node-2-config.toml", "127.0.0.1:4222", "nats://127.0.0.1:4221/,nats://127.0.0.1:4223/")
	node3 = startNode("examples/node-3-config.toml", "127.0.0.1:4223", "nats://127.0.0.1:4221/,nats://127.0.0.1:4222/")

	GinkgoWriter.Printf("Cluster started, waiting %v for stabilization\n", nodeStartupDelay*2)
	return node1, node2, node3
}

// stopNodes gracefully stops the provided HarmonyLite nodes.
func stopNodes(nodes ...*exec.Cmd) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Stopping nodes...\n")

	for _, node := range nodes {
		if node != nil && node.Process != nil {
			if err := node.Process.Kill(); err != nil {
				GinkgoWriter.Printf("Error killing node: %v\n", err)
			}
			// Use a closure to capture the node variable correctly in the Eventually
			func(n *exec.Cmd) {
				Eventually(n.Wait, nodeShutdownTimeout).ShouldNot(Succeed(), "Node should exit with error on kill")
			}(node)
		}
	}
	GinkgoWriter.Printf("Nodes stopped\n")
}

// startNode launches a HarmonyLite node and performs a NATS health check.
func startNode(config, addr, peers string) *exec.Cmd {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Starting node with config %s, addr %s, peers %s\n", config, addr, peers)

	wd, err := os.Getwd()
	Expect(err).To(BeNil())
	wd = wd[:len(wd)-len("/tests/e2e")]

	cmd := exec.Command(filepath.Join(wd, "harmonylite"), "-config", config, "-cluster-addr", addr, "-cluster-peers", peers)
	cmd.Dir = wd
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err = cmd.Start()
	Expect(err).To(BeNil(), "Failed to start node with config %s", config)

	// Perform NATS health check
	waitForNATSHealth(addr)

	return cmd
}

// waitForNATSHealth checks if the NATS cluster is healthy with retries.
func waitForNATSHealth(addr string) {
	natsURL := "nats://" + addr // Simplification: assumes NATS is on the same address
	var nc *nats.Conn
	var err error

	for i := 0; i < natsRetryAttempts; i++ {
		nc, err = nats.Connect(natsURL, nats.Timeout(natsConnectTimeout))
		if err == nil {
			break // Connected successfully
		}
		GinkgoWriter.Printf("NATS connection attempt %d failed: %v\n", i+1, err)
		backoff := natsRetryDelay * time.Duration(i+1) // Simple linear backoff
		if backoff > 16*time.Second {
			backoff = 16 * time.Second // Maximum backoff
		}
		time.Sleep(backoff)
	}
	Expect(err).To(BeNil(), "Failed to connect to NATS after retries")
	defer nc.Close()

	Eventually(func() bool {
		if nc.ConnectedServerId() == "" {
			GinkgoWriter.Printf("Waiting for server info\n")
			return false
		}
		GinkgoWriter.Printf("NATS Cluster Healthy\n")
		return true
	}, maxWaitTime, pollInterval).Should(BeTrue(), "NATS cluster did not become healthy")
}

// cleanup removes temporary files and directories.
func cleanup() {
	defer GinkgoRecover()
	patterns := []string{
		filepath.Join(dbDir, "harmonylite-1*"),
		filepath.Join(dbDir, "harmonylite-2*"),
		filepath.Join(dbDir, "harmonylite-3*"),
		filepath.Join(dbDir, "nats"), // Remove the nats directory as well
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			log.Printf("Error globbing %s: %v", pattern, err)
			continue
		}
		for _, file := range files {
			if err := os.RemoveAll(file); err != nil {
				log.Printf("Error removing %s: %v", file, err)
			}
		}
	}
	GinkgoWriter.Printf("Cleanup completed\n")
}

// -- Database Operations --

// createDatabase creates a SQLite database with the initial schema and data.
func createDatabase(dbPath string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Creating database: %s\n", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Failed to open database %s", dbPath)
	defer db.Close()

	_, err = db.Exec(`
		DROP TABLE IF EXISTS Books;
		CREATE TABLE Books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			publication_year INTEGER
		);
		INSERT INTO Books (title, author, publication_year)
		VALUES
		('The Hitchhiker''s Guide to the Galaxy', 'Douglas Adams', 1979),
		('The Lord of the Rings', 'J.R.R. Tolkien', 1954),
		('Harry Potter and the Sorcerer''s Stone', 'J.K. Rowling', 1997),
		('The Catcher in the Rye', 'J.D. Salinger', 1951),
		('To Kill a Mockingbird', 'Harper Lee', 1960),
		('1984', 'George Orwell', 1949),
		('The Great Gatsby', 'F. Scott Fitzgerald', 1925);
	`)
	Expect(err).To(BeNil(), "Failed to initialize database %s", dbPath)
}

// insertBook inserts a new book into the database.
func insertBook(dbPath, title, author string, year int) int64 {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Inserting %s into %s\n", title, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Error opening database %s", dbPath)
	defer db.Close()

	res, err := db.Exec(`INSERT INTO Books (title, author, publication_year) VALUES (?, ?, ?)`, title, author, year)
	Expect(err).To(BeNil(), "Error inserting book into %s", dbPath)
	id, err := res.LastInsertId()
	Expect(err).To(BeNil(), "Error getting last insert ID from %s", dbPath)
	return id
}

// updateBookTitle updates the title of a book.
func updateBookTitle(dbPath string, id int64, newTitle string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Updating ID %d to %s on %s\n", id, newTitle, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("UPDATE Books SET title = ? WHERE id = ?", newTitle, id)
	Expect(err).To(BeNil(), "Update failed on %s", dbPath)
}

// deleteBookByID deletes a book from the database by its ID.
func deleteBookByID(dbPath string, id int64) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Deleting ID %d from %s\n", id, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("DELETE FROM Books WHERE id = ?", id)
	Expect(err).To(BeNil(), "Delete failed on %s", dbPath)
}

// -- Assertions --

// getBookTitle retrieves the title of a book by its ID.  Returns an empty string if not found.
func getBookTitle(dbPath string, id int64) string {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	var title string
	err = db.QueryRow("SELECT title FROM Books WHERE id = ?", id).Scan(&title)
	if err == sql.ErrNoRows {
		return "" // Not found
	}
	Expect(err).To(BeNil(), "Error getting title for ID %d in %s", id, dbPath)
	return title
}

// countBooksByTitle counts the number of books with a given title.
func countBooksByTitle(dbPath, title string) int {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Error opening %s for count", dbPath)
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Books WHERE title = ?", title).Scan(&count)
	Expect(err).To(BeNil(), "Error counting title %s in %s", title, dbPath)
	GinkgoWriter.Printf("Counted %d '%s' in %s\n", count, title, dbPath)
	return count
}

// countBooksByID counts the number of books with a given ID.
func countBooksByID(dbPath string, id int64) int {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Books WHERE id = ?", id).Scan(&count)
	Expect(err).To(BeNil(), "Error counting ID %d in %s", id, dbPath)
	return count
}

// createAuthorsTable creates an Authors table in the specified database
func createAuthorsTable(dbPath string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Creating Authors table in: %s\n", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Failed to open database %s", dbPath)
	defer db.Close()

	_, err = db.Exec(`
		DROP TABLE IF EXISTS Authors;
		CREATE TABLE Authors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			birth_year INTEGER
		);
	`)
	Expect(err).To(BeNil(), "Failed to create Authors table in %s", dbPath)
}

// insertAuthor inserts a new author into the database
func insertAuthor(dbPath, name string, birthYear int) int64 {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Inserting author %s into %s\n", name, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Error opening database %s", dbPath)
	defer db.Close()

	res, err := db.Exec(`INSERT INTO Authors (name, birth_year) VALUES (?, ?)`, name, birthYear)
	Expect(err).To(BeNil(), "Error inserting author into %s", dbPath)
	id, err := res.LastInsertId()
	Expect(err).To(BeNil(), "Error getting last insert ID from %s", dbPath)
	return id
}

// countAuthorsByName counts the number of authors with a given name
func countAuthorsByName(dbPath, name string) int {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil(), "Error opening %s for count", dbPath)
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Authors WHERE name = ?", name).Scan(&count)
	Expect(err).To(BeNil(), "Error counting name %s in %s", name, dbPath)
	GinkgoWriter.Printf("Counted %d '%s' in %s\n", count, name, dbPath)
	return count
}

// countAuthorsByID counts the number of authors with a given ID
func countAuthorsByID(dbPath string, id int64) int {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Authors WHERE id = ?", id).Scan(&count)
	Expect(err).To(BeNil(), "Error counting ID %d in %s", id, dbPath)
	return count
}
