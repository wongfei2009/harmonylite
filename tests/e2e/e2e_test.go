package main_test

import (
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Marmot End-to-End Tests", Ordered, func() {
	var node1, node2, node3 *exec.Cmd

	BeforeAll(func() {
		node1, node2, node3 = startCluster()
	})

	AfterAll(func() {
		stopCluster(node1, node2, node3)
	})

	Context("Basic Replication", func() {
		It("should replicate INSERT operations", func() {
			id := insertBook(filepath.Join(dbDir, "marmot-1.db"), "Pride and Prejudice", "Jane Austen", 1813)
			Eventually(func() int {
				return countBooksByTitle(filepath.Join(dbDir, "marmot-2.db"), "Pride and Prejudice")
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert replication failed")
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Insert not replicated to node 3")
		})

		It("should replicate UPDATE operations", func() {
			id := insertBook(filepath.Join(dbDir, "marmot-1.db"), "Update Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for update")
			updateBookTitle(filepath.Join(dbDir, "marmot-1.db"), id, "Updated Title")
			Eventually(func() string {
				return getBookTitle(filepath.Join(dbDir, "marmot-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update replication failed")
			Eventually(func() string {
				return getBookTitle(filepath.Join(dbDir, "marmot-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal("Updated Title"), "Update not replicated to node 3")
		})

		It("should replicate DELETE operations", func() {
			id := insertBook(filepath.Join(dbDir, "marmot-1.db"), "Delete Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for delete")
			deleteBookByID(filepath.Join(dbDir, "marmot-1.db"), id)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete replication failed")
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-3.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(0), "Delete not replicated to node 3")
		})
	})

	Context("Concurrency and Conflict Resolution", func() {
		It("should resolve concurrent updates using last-writer-wins", func() {
			id := insertBook(filepath.Join(dbDir, "marmot-1.db"), "Conflict Test", "Author", 2020)
			Eventually(func() int {
				return countBooksByID(filepath.Join(dbDir, "marmot-2.db"), id)
			}, maxWaitTime, pollInterval).Should(Equal(1), "Initial insert not replicated for conflict test")

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				updateBookTitle(filepath.Join(dbDir, "marmot-1.db"), id, "Node1 Update")
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				time.Sleep(100 * time.Millisecond) // Ensure Node2 writes last
				updateBookTitle(filepath.Join(dbDir, "marmot-2.db"), id, "Node2 Update")
			}()
			wg.Wait()

			Eventually(func() bool {
				t1 := getBookTitle(filepath.Join(dbDir, "marmot-1.db"), id)
				t2 := getBookTitle(filepath.Join(dbDir, "marmot-2.db"), id)
				t3 := getBookTitle(filepath.Join(dbDir, "marmot-3.db"), id)
				consistent := t1 == "Node2 Update" && t2 == "Node2 Update" && t3 == "Node2 Update"
				GinkgoWriter.Printf("Conflict check - Node1: %s, Node2: %s, Node3: %s\n", t1, t2, t3)
				return consistent
			}, maxWaitTime, pollInterval).Should(BeTrue(), "Conflict resolution failed; expected 'Node2 Update' to win")
		})
	})
})
