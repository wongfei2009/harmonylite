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
	maxWaitTime         = 30 * time.Second       // Max time to wait for replication
	pollInterval        = 500 * time.Millisecond // Polling interval for Eventually
	nodeStartupDelay    = 10 * time.Second       // Initial delay for node startup
	nodeShutdownTimeout = 5 * time.Second        // Timeout for node shutdown
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

	// Wait for nodes to stabilize
	time.Sleep(nodeStartupDelay)
	GinkgoWriter.Printf("Cluster started, waiting %v for stabilization\n", nodeStartupDelay)

	return node1, node2, node3
}

// stopCluster gracefully stops the cluster
func stopCluster(node1, node2, node3 *exec.Cmd) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Stopping cluster...\n")

	for _, node := range []*exec.Cmd{node1, node2, node3} {
		if node.Process != nil {
			if err := node.Process.Kill(); err != nil {
				GinkgoWriter.Printf("Error killing node: %v\n", err)
			}
			Eventually(node.Wait, nodeShutdownTimeout).ShouldNot(Succeed(), "Node should exit with error on kill")
		}
	}
	cleanup()
	GinkgoWriter.Printf("Cluster stopped\n")
}

// createDB sets up a SQLite database with initial data
func createDB(dbFile string) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Creating database: %s\n", dbFile)
	db, err := sql.Open("sqlite3", dbFile)
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
	cmd := exec.Command(filepath.Join(wd, "marmot"), "-config", config, "-cluster-addr", addr, "-cluster-peers", peers)
	cmd.Dir = wd
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	err = cmd.Start()
	Expect(err).To(BeNil(), "Failed to start node with config %s", config)
	return cmd
}

// insertBook inserts a book and returns its ID
func insertBook(dbPath, title, author string, year int) int {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Inserting %s into %s\n", title, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
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
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("UPDATE Books SET title = ? WHERE id = ?", newTitle, id)
	Expect(err).To(BeNil(), "Update failed on %s", dbPath)
}

// deleteBook deletes a book by ID
func deleteBook(dbPath string, id int) {
	defer GinkgoRecover()
	GinkgoWriter.Printf("Deleting ID %d from %s\n", id, dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	_, err = db.Exec("DELETE FROM Books WHERE id = ?", id)
	Expect(err).To(BeNil(), "Delete failed on %s", dbPath)
}

// countBookByTitle counts occurrences of a title
func countBookByTitle(dbPath, title string) int {
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

// getBookTitleByID retrieves a book's title by ID
func getBookTitleByID(dbPath string, id int) string {
	defer GinkgoRecover()
	db, err := sql.Open("sqlite3", dbPath)
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
	db, err := sql.Open("sqlite3", dbPath)
	Expect(err).To(BeNil())
	defer db.Close()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM Books WHERE id = ?", id).Scan(&count)
	Expect(err).To(BeNil(), "Error counting ID %d in %s", id, dbPath)
	return count
}

var _ = Describe("Marmot", Ordered, func() {
	var node1, node2, node3 *exec.Cmd

	BeforeAll(func() {
		node1, node2, node3 = startCluster()
	})

	AfterAll(func() {
		stopCluster(node1, node2, node3)
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

	})
})
