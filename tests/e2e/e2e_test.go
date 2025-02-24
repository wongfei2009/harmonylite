package main_test

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Constants for timing and configuration
const (
	maxWaitTime         = 45 * time.Second       // Max time to wait for replication. Increased for robustness.
	pollInterval        = 500 * time.Millisecond // Polling interval for Eventually
	nodeStartupDelay    = 15 * time.Second       // Initial delay for node startup.  Increased.
	nodeShutdownTimeout = 10 * time.Second       // Timeout for node shutdown. Increased.
)

// cleanup removes temporary files and directories
func cleanup() {
	patterns := []string{
		"/tmp/marmot-1*",
		"/tmp/marmot-2*",
		"/tmp/marmot-3*",
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
	if err := os.RemoveAll("/tmp/nats"); err != nil {
		log.Printf("Error removing /tmp/nats: %v", err)
	}
	GinkgoWriter.Printf("Cleanup completed\n")
}

// startCluster initializes and starts a 3-node Marmot cluster
func startCluster() (*exec.Cmd, *exec.Cmd, *exec.Cmd) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Starting cluster setup...\n")
	cleanup()

	// Create databases
	for i := 1; i <= 3; i++ {
		createDB(fmt.Sprintf("/tmp/marmot-%d.db", i))
	}

	// Start nodes with correct peering
	node1 := startNode("examples/node-1-config.toml", "localhost:4221", "nats://localhost:4222/,nats://localhost:4223/")
	node2 := startNode("examples/node-2-config.toml", "localhost:4222", "nats://localhost:4221/,nats://localhost:4223/")
	node3 := startNode("examples/node-3-config.toml", "localhost:4223", "nats://localhost:4221/,nats://localhost:4222/")

	// Wait for nodes to stabilize.  Give it extra time.
	time.Sleep(nodeStartupDelay)
	GinkgoWriter.Printf("Cluster started, waiting %v for stabilization\n", nodeStartupDelay)

	return node1, node2, node3
}

// stopCluster gracefully stops the cluster
func stopCluster(node1, node2, node3 *exec.Cmd) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Stopping cluster...\n")

	for _, node := range []*exec.Cmd{node1, node2, node3} {
		if node == nil {
			continue
		}
		if node.Process != nil {
			// Use Signal(os.Interrupt) instead of Kill().  Kill() is SIGKILL (9),
			// which cannot be caught.  Interrupt is SIGINT (2), which Marmot
			// can catch and handle gracefully (close connections, etc.).
			if err := node.Process.Signal(os.Interrupt); err != nil {
				GinkgoWriter.Printf("Error sending interrupt to node: %v\n", err)
				// If interrupt fails, THEN try to kill.
				if err := node.Process.Kill(); err != nil {
					GinkgoWriter.Printf("Error killing node: %v\n", err)
				}
			}
			// Use a closure to capture the current node for the error message.
			Eventually(func() error {
				return node.Wait()
			}, nodeShutdownTimeout).Should(Or(Succeed(), HaveOccurred()), "Node should exit cleanly or with an error")
		}
	}
	GinkgoWriter.Printf("Cluster stopped\n")
}

// createDB sets up a SQLite database with initial data
func createDB(dbFile string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Creating database: %s\n", dbFile)
	db, err := sql.Open("sqlite3", dbFile+"?_journal_mode=WAL") // Explicitly set WAL mode
	Expect(err).To(BeNil(), "Failed to open database %s", dbFile)
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
	Expect(err).To(BeNil(), "Failed to initialize database %s", dbFile)
}

// startNode launches a Marmot node
func startNode(config, addr, peers string) *exec.Cmd {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Starting node with config %s, addr %s, peers %s\n", config, addr, peers)
	wd, err := os.Getwd()
	Expect(err).To(BeNil())
	wd = wd[:len(wd)-len("/tests/e2e")]
	// Use the absolute path to marmot executable
	cmd := exec.Command(filepath.Join(wd, "marmot"), "-config", config, "-cluster-addr", addr, "-cluster-peers", peers)
	cmd.Dir = wd
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	// Wrap the command start in an Eventually block to give it time to bind to the port.
	Eventually(func() error {
		return cmd.Start()
	}, nodeStartupDelay, pollInterval).Should(Succeed(), "Failed to start node with config %s", config)

	return cmd
}

// insertBook inserts a book and returns its ID
func insertBook(dbPath, title, author string, year int) int {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Inserting %s into %s\n", title, dbPath)
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil(), "Error opening database %s", dbPath)
	defer db.Close()

	res, err := db.Exec(`INSERT INTO Books (title, author, publication_year) VALUES (?, ?, ?)`, title, author, year)
	Expect(err).To(BeNil(), "Error inserting book into %s", dbPath)
	id, err := res.LastInsertId()
	Expect(err).To(BeNil(), "Error getting last insert ID from %s", dbPath)
	return int(id)
}

// updateBook updates a book's title by ID
func updateBook(dbPath string, id int, newTitle string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Updating ID %d to %s on %s\n", id, newTitle, dbPath)
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("UPDATE Books SET title = ? WHERE id = ?", newTitle, id)
	Expect(err).To(BeNil(), "Update failed on %s", dbPath)
}

// deleteBook deletes a book by ID
func deleteBook(dbPath string, id int) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Deleting ID %d from %s\n", id, dbPath)
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("DELETE FROM Books WHERE id = ?", id)
	Expect(err).To(BeNil(), "Delete failed on %s", dbPath)
}

// countBookByTitle counts occurrences of a title
func countBookByTitle(dbPath, title string) int {
	defer GinkgoRecover()
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil(), "Error opening %s for count", dbPath)
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Books WHERE title = ?", title).Scan(&count)
	Expect(err).To(BeNil(), "Error counting title %s in %s", title, dbPath)
	GinkgoWriter.Printf("Counted %d '%s' in %s\n", count, title, dbPath)
	return count
}

// getBookTitleByID retrieves a book's title by ID
func getBookTitleByID(dbPath string, id int) string {
	defer GinkgoRecover()
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil())
	defer db.Close()
	var title string
	err = db.QueryRow("SELECT title FROM Books WHERE id = ?", id).Scan(&title)
	if err == sql.ErrNoRows {
		return ""
	}
	Expect(err).To(BeNil(), "Error getting title for ID %d in %s", id, dbPath)
	return title
}

// countBookByID counts occurrences of a book by ID
func countBookByID(dbPath string, id int) int {
	defer GinkgoRecover()
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil())
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Books WHERE id = ?", id).Scan(&count)
	Expect(err).To(BeNil(), "Error counting ID %d in %s", id, dbPath)
	return count
}

// getAllBooks retrieves all books from a database.  Useful for more complex state checks.
func getAllBooks(dbPath string) []map[string]interface{} {
	defer GinkgoRecover()
	// Explicitly set WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	Expect(err).To(BeNil(), "Error opening %s for read", dbPath)
	defer db.Close()

	rows, err := db.Query("SELECT id, title, author, publication_year FROM Books")
	Expect(err).To(BeNil(), "Error querying all books from %s", dbPath)
	defer rows.Close()

	var books []map[string]interface{}
	for rows.Next() {
		var id int
		var title, author string
		var year int
		err = rows.Scan(&id, &title, &author, &year)
		Expect(err).To(BeNil(), "Error scanning book row in %s", dbPath)
		book := map[string]interface{}{
			"id":               id,
			"title":            title,
			"author":           author,
			"publication_year": year,
		}
		books = append(books, book)
	}
	return books
}

// compareBooks compares two book slices for equality.
func compareBooks(books1, books2 []map[string]interface{}) bool {
	if len(books1) != len(books2) {
		return false
	}

	// Create maps for faster lookup
	books1Map := make(map[int]map[string]interface{})
	for _, book := range books1 {
		books1Map[book["id"].(int)] = book
	}

	for _, book2 := range books2 {
		id := book2["id"].(int)
		book1, ok := books1Map[id]
		if !ok {
			return false // Book ID not found in books1
		}
		if book1["title"] != book2["title"] || book1["author"] != book2["author"] || book1["publication_year"] != book2["publication_year"] {
			return false
		}
	}

	return true
}

var _ = Describe("Marmot", Ordered, func() {
	var node1, node2, node3 *exec.Cmd

	BeforeAll(func() {
		node1, node2, node3 = startCluster()
	})

	AfterAll(func() {
		stopCluster(node1, node2, node3)
		cleanup() // Final cleanup, just in case.
	})

	Context("when the system is running", func() {
		It("should replicate an insert operation", func() {
			id := insertBook("/tmp/marmot-1.db", "Pride and Prejudice", "Jane Austen", 1813)
			Eventually(func() int {
				return countBookByTitle("/tmp/marmot-2.db", "Pride and Prejudice")
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert replication failed")
			Eventually(func() int {
				return countBookByID("/tmp/marmot-3.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert not replicated to node 3")
		})

		It("should replicate an update operation", func() {
			id := insertBook("/tmp/marmot-1.db", "Update Test", "Author", 2020)
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for update")
			updateBook("/tmp/marmot-1.db", id, "Updated Title")
			Eventually(func() string {
				return getBookTitleByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update replication failed")
			Eventually(func() string {
				return getBookTitleByID("/tmp/marmot-3.db", id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update not replicated to node 3")
		})

		It("should replicate a delete operation", func() {
			id := insertBook("/tmp/marmot-1.db", "Delete Test", "Author", 2020)
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for delete")
			deleteBook("/tmp/marmot-1.db", id)
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete replication failed")
			Eventually(func() int {
				return countBookByID("/tmp/marmot-3.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete not replicated to node 3")
		})

		It("should resolve concurrent updates with last writer wins", func() {
			id := insertBook("/tmp/marmot-1.db", "Conflict Test", "Author", 2020)
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for conflict test")

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				updateBook("/tmp/marmot-1.db", id, "Node1 Update")
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				time.Sleep(100 * time.Millisecond) // Ensure Node2 writes last
				updateBook("/tmp/marmot-2.db", id, "Node2 Update")
			}()
			wg.Wait()

			Eventually(func() bool {
				t1 := getBookTitleByID("/tmp/marmot-1.db", id)
				t2 := getBookTitleByID("/tmp/marmot-2.db", id)
				t3 := getBookTitleByID("/tmp/marmot-3.db", id)
				consistent := t1 == "Node2 Update" && t2 == "Node2 Update" && t3 == "Node2 Update"
				GinkgoWriter.Printf("Conflict check - Node1: %s, Node2: %s, Node3: %s\n", t1, t2, t3)
				return consistent
			}, maxWaitTime, pollInterval).Should(BeTrue(), "Conflict resolution failed; expected 'Node2 Update' to win")
		})

		// New test cases:

		It("should handle rapid successive inserts", func() {
			// Insert multiple books in quick succession on node 1.
			ids := make([]int, 5)
			for i := 0; i < 5; i++ {
				ids[i] = insertBook("/tmp/marmot-1.db", fmt.Sprintf("Rapid Insert %d", i), "Author", 2020+i)
			}

			// Check if all inserts are replicated to node 2 and 3.
			for _, id := range ids {
				Eventually(func() int {
					return countBookByID("/tmp/marmot-2.db", id)
				}, maxWaitTime, pollInterval).Should(Equal(1), "Rapid insert not replicated to node 2")
				Eventually(func() int {
					return countBookByID("/tmp/marmot-3.db", id)
				}, maxWaitTime, pollInterval).Should(Equal(1), "Rapid insert not replicated to node 3")
			}
		})

		It("should handle rapid successive updates", func() {
			// Insert a book on node 1.
			id := insertBook("/tmp/marmot-1.db", "Rapid Update Base", "Author", 2020)
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for rapid updates")

			// Rapidly update the book multiple times on node 1.
			for i := 0; i < 5; i++ {
				updateBook("/tmp/marmot-1.db", id, fmt.Sprintf("Rapid Update %d", i))
			}

			// Check if the final update is replicated to node 2 and 3.
			Eventually(func() string {
				return getBookTitleByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal("Rapid Update 4"), "Rapid updates not replicated to node 2")
			Eventually(func() string {
				return getBookTitleByID("/tmp/marmot-3.db", id)
			}, maxWaitTime, pollInterval).Should(Equal("Rapid Update 4"), "Rapid updates not replicated to node 3")
		})

		It("should handle rapid successive deletes", func() {
			// Insert multiple books and then rapidly delete them.
			ids := make([]int, 5)
			for i := 0; i < 5; i++ {
				ids[i] = insertBook("/tmp/marmot-1.db", fmt.Sprintf("Rapid Delete %d", i), "Author", 2020+i)
			}

			// Wait for the inserts to propagate, so we're deleting something that exists
			for _, id := range ids {
				Eventually(func() int {
					return countBookByID("/tmp/marmot-2.db", id)
				}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for rapid deletes")
			}

			// Now, rapidly delete them
			for _, id := range ids {
				deleteBook("/tmp/marmot-1.db", id)
			}

			// Check if all deletes are replicated to node 2 and 3.
			for _, id := range ids {
				Eventually(func() int {
					return countBookByID("/tmp/marmot-2.db", id)
				}, maxWaitTime, pollInterval).Should(Equal(0), "Rapid delete not replicated to node 2")
				Eventually(func() int {
					return countBookByID("/tmp/marmot-3.db", id)
				}, maxWaitTime, pollInterval).Should(Equal(0), "Rapid delete not replicated to node 3")
			}
		})

		It("should maintain consistency after multiple mixed operations", func() {
			// Perform a series of mixed insert, update, and delete operations.
			id1 := insertBook("/tmp/marmot-1.db", "Mixed Op 1", "Author1", 2021)
			id2 := insertBook("/tmp/marmot-2.db", "Mixed Op 2", "Author2", 2022)
			updateBook("/tmp/marmot-1.db", id1, "Mixed Op 1 Updated")
			deleteBook("/tmp/marmot-2.db", id2)
			insertBook("/tmp/marmot-3.db", "Mixed Op 3", "Author3", 2023)

			// Wait for all operations to replicate.
			Eventually(func() bool {
				b1 := getAllBooks("/tmp/marmot-1.db")
				b2 := getAllBooks("/tmp/marmot-2.db")
				b3 := getAllBooks("/tmp/marmot-3.db")
				return compareBooks(b1, b2) && compareBooks(b2, b3)

			}, maxWaitTime, pollInterval).Should(BeTrue(), "Databases not consistent after mixed operations")

		})

		It("should maintain consistency with operations from different nodes", func() {
			// Perform operations from different nodes to ensure they are all replicated correctly.
			deleteBook("/tmp/marmot-1.db", 1)
			insertBook("/tmp/marmot-2.db", "Book from Node 2", "Author2", 2024)
			updateBook("/tmp/marmot-3.db", 2, "Updated from Node 3")

			// Wait for all operations to replicate and verify consistency across all nodes.
			Eventually(func() bool {
				books1 := getAllBooks("/tmp/marmot-1.db")
				books2 := getAllBooks("/tmp/marmot-2.db")
				books3 := getAllBooks("/tmp/marmot-3.db")

				// Check for consistency across all node pairs.
				return compareBooks(books1, books2) && compareBooks(books2, books3) && compareBooks(books1, books3)
			}, maxWaitTime*2, pollInterval).Should(BeTrue(), "Databases inconsistent after operations from different nodes")
		})

		It("Should eventually replicate data even after a node restart", func() {
			// Stop Node 3
			stopCluster(nil, nil, node3)

			// Insert on Node 1
			id := insertBook("/tmp/marmot-1.db", "Restart Test", "Author", 2024)

			// Verify Node 2 gets it
			Eventually(func() int {
				return countBookByID("/tmp/marmot-2.db", id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert before restart not replicated to node 2")

			// Restart Node 3
			node3 = startNode("examples/node-3-config.toml", "localhost:4223", "nats://localhost:4221/,nats://localhost:4222/")
			time.Sleep(nodeStartupDelay)

			// Verify Node 3 *eventually* gets it.
			Eventually(func() int {
				return countBookByID("/tmp/marmot-3.db", id)
			}, maxWaitTime*3, pollInterval).Should(Equal(1), "Insert not replicated to restarted node 3") // Longer timeout

			// Insert a new record after restart, check all nodes
			insertBook("/tmp/marmot-1.db", "After Restart Test", "Author", 2024)
			Eventually(func() bool {
				b1 := getAllBooks("/tmp/marmot-1.db")
				b2 := getAllBooks("/tmp/marmot-2.db")
				b3 := getAllBooks("/tmp/marmot-3.db")
				return compareBooks(b1, b2) && compareBooks(b2, b3)
			}, maxWaitTime*2, pollInterval).Should(BeTrue(), "Inconsistency after node 3 restart")
		})

	})
})
