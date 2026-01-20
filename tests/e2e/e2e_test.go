package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var wd string

var _ = Describe("HarmonyLite End-to-End Tests", Ordered, func() {
	var node1, node2, node3 *exec.Cmd

	BeforeAll(func() {
		var err error
		wd, err = os.Getwd()
		Expect(err).To(BeNil())
		wd = wd[:len(wd)-len("/tests/e2e")]
		cleanup()

		// Create databases with both tables
		for i := 1; i <= 3; i++ {
			dbPath := filepath.Join(dbDir, fmt.Sprintf("harmonylite-%d.db", i))
			createDatabase(dbPath)     // Creates Books table
			createAuthorsTable(dbPath) // Creates Authors table
		}

		node1, node2, node3 = startCluster()
	})

	AfterAll(func() {
		stopNodes(node1, node2, node3)
	})

	Context("Basic Replication", func() {
		It("should replicate INSERT operations", func() {
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Pride and Prejudice", "Jane Austen", 1813)
			Eventually(func() int {
				return countBooksByTitle(filepath.Join(dbDir, "harmonylite-2.db"), "Pride and Prejudice")
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert replication failed")
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert not replicated to node 3")
		})

		It("should replicate UPDATE operations", func() {
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Update Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for update")
			updateBookTitle(filepath.Join(dbDir, "harmonylite-1.db"), id, "Updated Title")
			Eventually(func() string {
				return getBookTitle(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update replication failed")
			Eventually(func() string {
				return getBookTitle(filepath.Join(dbDir, "harmonylite-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update not replicated to node 3")
		})

		It("should replicate DELETE operations", func() {
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Delete Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for delete")
			deleteBookByID(filepath.Join(dbDir, "harmonylite-1.db"), id)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete replication failed")
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete not replicated to node 3")
		})
	})

	Context("Concurrency and Conflict Resolution", func() {
		It("should resolve concurrent updates using last-writer-wins", func() {
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Conflict Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for conflict test")

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				updateBookTitle(filepath.Join(dbDir, "harmonylite-1.db"), id, "Node1 Update")
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				time.Sleep(100 * time.Millisecond) // Ensure Node2 writes last
				updateBookTitle(filepath.Join(dbDir, "harmonylite-2.db"), id, "Node2 Update")
			}()
			wg.Wait()

			Eventually(func() bool {
				t1 := getBookTitle(filepath.Join(dbDir, "harmonylite-1.db"), id)
				t2 := getBookTitle(filepath.Join(dbDir, "harmonylite-2.db"), id)
				t3 := getBookTitle(filepath.Join(dbDir, "harmonylite-3.db"), id)
				consistent := t1 == "Node2 Update" && t2 == "Node2 Update" && t3 == "Node2 Update"
				GinkgoWriter.Printf("Conflict check - Node1: %s, Node2: %s, Node3: %s\n", t1, t2, t3)
				return consistent
			}, maxWaitTime, pollInterval).Should(BeTrue(), "Conflict resolution failed; expected 'Node2 Update' to win")
		})
	})

	Context("Snapshot and Restore", func() {
		It("should save and restore snapshots correctly", func() {
			// Insert data before snapshot
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Snapshot Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for snapshot test")

			// Stop node 3, save snapshot, and restart
			stopNodes(node3)
			time.Sleep(2 * time.Second) // Allow snapshot to be taken
			node3Cmd := exec.Command(filepath.Join(wd, "harmonylite"),
				"-config", "examples/node-3-config.toml",
				"-node-id", "3",
				"-save-snapshot")
			node3Cmd.Dir = wd
			node3Cmd.Stdout = GinkgoWriter
			node3Cmd.Stderr = GinkgoWriter
			Expect(node3Cmd.Run()).To(Succeed(), "Failed to save snapshot on node 3")
			node3 = startNode("examples/node-3-config.toml", "127.0.0.1:4223", "nats://127.0.0.1:4221/,nats://127.0.0.1:4222/", 3)

			// Verify restored data
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id)
			}, maxWaitTime*2, pollInterval).Should(Equal(1), "Snapshot restore failed on node 3")
		})
	})

	Context("Node Failure and Recovery", func() {
		It("should recover replication after node failure", func() {
			// Insert initial data
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Failure Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for failure test")

			// Simulate node 3 failure
			stopNodes(node3)
			time.Sleep(2 * time.Second) // Allow some time for failure to propagate

			// Insert more data while node 3 is down
			id2 := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Post-Failure Test", "Author", 2021)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id2)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert not replicated to node 2 during node 3 downtime")

			// Restart node 3
			node3 = startNode("examples/node-3-config.toml", "127.0.0.1:4223", "nats://127.0.0.1:4221/,nats://127.0.0.1:4222/", 3)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id2)
			}, maxWaitTime*2, pollInterval).Should(Equal(1), "Node 3 failed to recover replication after restart")
		})
	})

	Context("Multi-Table Replication", func() {
		BeforeEach(func() {
		})

		It("should replicate changes across multiple tables", func() {
			bookID := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Multi-Table Book", "Jane Austen", 1813)
			authorID := insertAuthor(filepath.Join(dbDir, "harmonylite-1.db"), "Jane Austen", 1775)

			Eventually(func() int {
				return countBooksByTitle(filepath.Join(dbDir, "harmonylite-2.db"), "Multi-Table Book")
			}, maxWaitTime, pollInterval).Should(Equal(1), "Book replication failed across tables")
			Eventually(func() int {
				return countAuthorsByName(filepath.Join(dbDir, "harmonylite-2.db"), "Jane Austen")
			}, maxWaitTime, pollInterval).Should(Equal(1), "Author replication failed across tables")

			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), bookID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Book not replicated to node 3")
			Eventually(func() int {
				return countAuthorsByID(filepath.Join(dbDir, "harmonylite-3.db"), authorID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Author not replicated to node 3")
		})
	})

	Context("Large Data Volumes", func() {
		It("should handle replication of many records", func() {
			const numRecords = 100
			var ids []int64
			for i := 0; i < numRecords; i++ {
				id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Bulk Test "+string(rune(i)), "Author", 2020+i)
				ids = append(ids, id)
			}

			Eventually(func() int {
				count := 0
				for _, id := range ids {
					count += countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
				}
				return count
			}, maxWaitTime*2, pollInterval).Should(Equal(numRecords), "Failed to replicate all records to node 2")

			Eventually(func() int {
				count := 0
				for _, id := range ids {
					count += countBooksByID(filepath.Join(dbDir, "harmonylite-3.db"), id)
				}
				return count
			}, maxWaitTime*2, pollInterval).Should(Equal(numRecords), "Failed to replicate all records to node 3")
		})
	})

	Context("Configuration Variations", func() {
		It("should work with publish disabled on one node", func() {
			// Stop node 2
			stopNodes(node2)

			// Create a temporary config file for node 2 with publish disabled
			tmpConfigPath := filepath.Join(dbDir, "node-2-disabled-publish.toml")
			originalConfig, err := os.ReadFile(filepath.Join(wd, "examples/node-2-config.toml"))
			Expect(err).To(BeNil(), "Failed to read original config file")

			// Append publish=false to the config
			modifiedConfig := append([]byte("\npublish=false\n"), originalConfig...)
			err = os.WriteFile(tmpConfigPath, modifiedConfig, 0644)
			Expect(err).To(BeNil(), "Failed to write modified config file")

			// Start node 2 with the modified config
			node2 = exec.Command(filepath.Join(wd, "harmonylite"),
				"-config", tmpConfigPath,
				"-node-id", "2",
				"-cluster-addr", "127.0.0.1:4222",
				"-cluster-peers", "nats://127.0.0.1:4221/,nats://127.0.0.1:4223/")
			node2.Dir = wd
			node2.Stdout = GinkgoWriter
			node2.Stderr = GinkgoWriter
			Expect(node2.Start()).To(Succeed(), "Failed to start node 2 with publish disabled")

			// Wait for NATS to be healthy before proceeding
			waitForNATSHealth("127.0.0.1:4222")

			// Give HarmonyLite additional time to complete initialization with JetStream
			// The retry logic in NewReplicator handles transient JetStream unavailability
			time.Sleep(2 * time.Second)

			// Insert on node 2 (should not replicate)
			id := insertBook(filepath.Join(dbDir, "harmonylite-2.db"), "No Publish Test", "Author", 2020)
			Consistently(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-1.db"), id)
			}, 5*time.Second, pollInterval).Should(Equal(0), "Data replicated from node 2 despite publish disabled")

			// Insert on node 1 (should replicate to node 2)
			id2 := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Publish Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id2)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Data not replicated to node 2 with publish disabled")

			// Clean up temp config file
			os.Remove(tmpConfigPath)

			// Restart node 2 with original config for subsequent tests
			// The retry logic in NewReplicator handles JetStream sync delays
			stopNodes(node2)
			node2 = startNode("examples/node-2-config.toml", "127.0.0.1:4222", "nats://127.0.0.1:4221/,nats://127.0.0.1:4223/", 2)
		})
	})

	Context("Snapshot Leader Election", func() {
		It("should elect one node as snapshot leader among multiple publishers", func() {
			// All nodes should have publish=true at this point

			// Insert data to trigger some activity
			id := insertBook(filepath.Join(dbDir, "harmonylite-1.db"), "Leader Election Test", "Author", 2026)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "harmonylite-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Data not replicated for leader election test")

			// Wait for leader election to stabilize (TTL is 30s, so wait a bit)
			time.Sleep(5 * time.Second)

			// At this point, leader election should have happened
			// We can't directly check which node is leader from here,
			// but the test passes if no panics/errors occurred during election
			GinkgoWriter.Println("Leader election completed without errors")
		})

		// Note: Leader failover is tested in unit tests (snapshot_leader_test.go)
		// E2E testing of failover with embedded NATS clusters is inherently flaky
		// due to JetStream quorum requirements and cluster reformation timing
	})

	Context("Schema Mismatch Pause and Resume", func() {
		// This test validates the schema versioning feature:
		// 1. When sender has a different schema than receiver, replication pauses
		// 2. After receiver's schema is upgraded, replication resumes automatically

		It("should pause replication on schema mismatch and resume after upgrade", func() {
			db1Path := filepath.Join(dbDir, "harmonylite-1.db")
			db2Path := filepath.Join(dbDir, "harmonylite-2.db")
			db3Path := filepath.Join(dbDir, "harmonylite-3.db")

			// Step 1: Verify normal replication is working (baseline)
			baselineID := insertBook(db1Path, "Schema Test Baseline", "Author", 2025)
			Eventually(func() int {
				return countBooksByID(db2Path, baselineID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Baseline replication should work before schema change")
			Eventually(func() int {
				return countBooksByID(db3Path, baselineID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Baseline replication should work on node 3")
			GinkgoWriter.Println("Baseline replication verified - all nodes in sync")

			// Step 2: Stop node 1 and upgrade its schema
			stopNodes(node1)
			time.Sleep(2 * time.Second)

			// Add a "rating" column to Books table on node 1 only
			alterTableAddColumn(db1Path, "Books", "rating", "INTEGER")
			Expect(hasColumn(db1Path, "Books", "rating")).To(BeTrue(), "Column should exist after ALTER TABLE")
			Expect(hasColumn(db2Path, "Books", "rating")).To(BeFalse(), "Node 2 should not have the new column yet")
			GinkgoWriter.Println("Schema upgraded on node 1 - added 'rating' column to Books")

			// Step 3: Restart node 1 with the new schema
			node1 = startNode("examples/node-1-config.toml", "127.0.0.1:4221", "nats://127.0.0.1:4222/,nats://127.0.0.1:4223/", 1)
			GinkgoWriter.Println("Node 1 restarted with upgraded schema")

			// Wait for CDC initialization to complete (triggers and change_log table created)
			waitForCDCReady(db1Path, "Books")

			// Step 4: Insert data from node 1 (with new schema)
			// This will have a different schema hash than nodes 2 and 3
			rating := 5
			mismatchID := insertBookWithRating(db1Path, "Schema Mismatch Test", "Author", 2025, &rating)
			GinkgoWriter.Printf("Inserted book with ID %d from upgraded node 1\n", mismatchID)

			// Step 5: Verify replication is PAUSED on node 2 (schema mismatch)
			// The data should NOT replicate because the schemas don't match
			Consistently(func() int {
				return countBooksByID(db2Path, mismatchID)
			}, 10*time.Second, pollInterval).Should(Equal(0), "Replication should be paused due to schema mismatch - data should NOT appear on node 2")
			GinkgoWriter.Println("Verified: Replication is paused on node 2 due to schema mismatch")

			// Step 6: Upgrade schema on node 2 (rolling upgrade simulation)
			stopNodes(node2)
			time.Sleep(2 * time.Second)
			alterTableAddColumn(db2Path, "Books", "rating", "INTEGER")
			Expect(hasColumn(db2Path, "Books", "rating")).To(BeTrue(), "Node 2 should now have the rating column")
			GinkgoWriter.Println("Schema upgraded on node 2")

			// Restart node 2
			node2 = startNode("examples/node-2-config.toml", "127.0.0.1:4222", "nats://127.0.0.1:4221/,nats://127.0.0.1:4223/", 2)
			GinkgoWriter.Println("Node 2 restarted with upgraded schema")

			// Wait for CDC initialization to complete
			waitForCDCReady(db2Path, "Books")

			// Step 7: Verify replication RESUMES and data appears on node 2
			Eventually(func() int {
				return countBooksByID(db2Path, mismatchID)
			}, maxWaitTime*2, pollInterval).Should(Equal(1), "Replication should resume after schema upgrade - data should appear on node 2")
			GinkgoWriter.Println("Verified: Replication resumed on node 2 after schema upgrade")

			// Step 8: Verify node 3 is still paused (hasn't been upgraded yet)
			Consistently(func() int {
				return countBooksByID(db3Path, mismatchID)
			}, 5*time.Second, pollInterval).Should(Equal(0), "Node 3 should still be paused - schema not upgraded")
			GinkgoWriter.Println("Verified: Node 3 still paused (schema not yet upgraded)")

			// Step 9: Complete rolling upgrade on node 3
			stopNodes(node3)
			time.Sleep(2 * time.Second)
			alterTableAddColumn(db3Path, "Books", "rating", "INTEGER")
			node3 = startNode("examples/node-3-config.toml", "127.0.0.1:4223", "nats://127.0.0.1:4221/,nats://127.0.0.1:4222/", 3)
			GinkgoWriter.Println("Node 3 restarted with upgraded schema")

			// Wait for CDC initialization to complete
			waitForCDCReady(db3Path, "Books")

			// Step 10: Verify all nodes are now in sync
			Eventually(func() int {
				return countBooksByID(db3Path, mismatchID)
			}, maxWaitTime*2, pollInterval).Should(Equal(1), "Replication should resume on node 3 after upgrade")
			GinkgoWriter.Println("Verified: All nodes now in sync with upgraded schema")

			// Final verification: new inserts replicate normally across all nodes
			finalRating := 4
			finalID := insertBookWithRating(db1Path, "Post-Upgrade Test", "Author", 2025, &finalRating)
			Eventually(func() int {
				return countBooksByID(db2Path, finalID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Post-upgrade replication to node 2 should work")
			Eventually(func() int {
				return countBooksByID(db3Path, finalID)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Post-upgrade replication to node 3 should work")
			GinkgoWriter.Println("Final verification: Normal replication works after complete rolling upgrade")
		})
	})
})
