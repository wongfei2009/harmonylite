package logstream

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestNatsServer starts an embedded NATS server for testing
func startTestNatsServer(t *testing.T) (*server.Server, *nats.Conn) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	require.NoError(t, err)

	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	return ns, nc
}

func TestSchemaRegistry_PublishAndGet(t *testing.T) {
	ns, nc := startTestNatsServer(t)
	defer ns.Shutdown()
	defer nc.Close()

	registry, err := NewSchemaRegistry(nc, 1)
	require.NoError(t, err)

	// Publish schema state
	testHash := "abc123def456"
	err = registry.PublishSchemaState(testHash, "")
	require.NoError(t, err)

	// Give it a moment to propagate
	time.Sleep(100 * time.Millisecond)

	// Retrieve schema state
	state, err := registry.GetNodeSchemaState(1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), state.NodeId)
	assert.Equal(t, testHash, state.SchemaHash)
	assert.Empty(t, state.PreviousHash)
	assert.NotEmpty(t, state.HarmonyLiteVersion)
	assert.False(t, state.UpdatedAt.IsZero())
}

func TestSchemaRegistry_ClusterState(t *testing.T) {
	ns, nc := startTestNatsServer(t)
	defer ns.Shutdown()
	defer nc.Close()

	// Create two registries for different nodes
	registry1, err := NewSchemaRegistry(nc, 1)
	require.NoError(t, err)

	registry2, err := NewSchemaRegistry(nc, 2)
	require.NoError(t, err)

	// Publish schema states
	hash1 := "abc123"
	hash2 := "def456"
	err = registry1.PublishSchemaState(hash1, "")
	require.NoError(t, err)

	err = registry2.PublishSchemaState(hash2, "")
	require.NoError(t, err)

	// Give it a moment to propagate
	time.Sleep(100 * time.Millisecond)

	// Get cluster state
	states, err := registry1.GetClusterSchemaState()
	require.NoError(t, err)
	assert.Len(t, states, 2)

	state1, ok := states[1]
	require.True(t, ok)
	assert.Equal(t, hash1, state1.SchemaHash)

	state2, ok := states[2]
	require.True(t, ok)
	assert.Equal(t, hash2, state2.SchemaHash)
}

func TestSchemaRegistry_ConsistencyCheck(t *testing.T) {
	t.Run("consistent schemas", func(t *testing.T) {
		ns, nc := startTestNatsServer(t)
		defer ns.Shutdown()
		defer nc.Close()

		registry1, err := NewSchemaRegistry(nc, 10)
		require.NoError(t, err)

		registry2, err := NewSchemaRegistry(nc, 20)
		require.NoError(t, err)

		// Both nodes have the same schema
		sameHash := "matching123"
		err = registry1.PublishSchemaState(sameHash, "")
		require.NoError(t, err)

		err = registry2.PublishSchemaState(sameHash, "")
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		report, err := registry1.CheckClusterSchemaConsistency()
		require.NoError(t, err)
		assert.True(t, report.Consistent)
		assert.Empty(t, report.Mismatches)
		assert.Equal(t, 2, report.NodeCount)
	})

	t.Run("inconsistent schemas", func(t *testing.T) {
		ns, nc := startTestNatsServer(t)
		defer ns.Shutdown()
		defer nc.Close()

		registry3, err := NewSchemaRegistry(nc, 30)
		require.NoError(t, err)

		registry4, err := NewSchemaRegistry(nc, 40)
		require.NoError(t, err)

		// Nodes have different schemas
		hash3 := "different1"
		hash4 := "different2"
		err = registry3.PublishSchemaState(hash3, "")
		require.NoError(t, err)

		err = registry4.PublishSchemaState(hash4, "")
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		report, err := registry3.CheckClusterSchemaConsistency()
		require.NoError(t, err)
		assert.False(t, report.Consistent)
		assert.Len(t, report.Mismatches, 1)
		assert.Equal(t, 2, report.NodeCount)
	})
}

func TestSchemaRegistry_TTL(t *testing.T) {
	ns, nc := startTestNatsServer(t)
	defer ns.Shutdown()
	defer nc.Close()

	registry, err := NewSchemaRegistry(nc, 100)
	require.NoError(t, err)

	// Publish schema state
	err = registry.PublishSchemaState("test-hash", "")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify it exists
	states, err := registry.GetClusterSchemaState()
	require.NoError(t, err)
	assert.Len(t, states, 1)

	// Note: Testing actual TTL expiry would require waiting 5 minutes
	// which is impractical for unit tests. The TTL is configured correctly
	// in NewSchemaRegistry, so we just verify the entry was created.
}

func TestSchemaRegistry_PreviousHash(t *testing.T) {
	ns, nc := startTestNatsServer(t)
	defer ns.Shutdown()
	defer nc.Close()

	registry, err := NewSchemaRegistry(nc, 1)
	require.NoError(t, err)

	// Publish schema state with previous hash (simulating rolling upgrade)
	currentHash := "new-schema-hash"
	previousHash := "old-schema-hash"
	err = registry.PublishSchemaState(currentHash, previousHash)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Retrieve schema state and verify both hashes are present
	state, err := registry.GetNodeSchemaState(1)
	require.NoError(t, err)
	assert.Equal(t, currentHash, state.SchemaHash)
	assert.Equal(t, previousHash, state.PreviousHash)
}
