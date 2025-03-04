package logstream

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/wongfei2009/harmonylite/cfg"
)

func TestReplicationState_Init(t *testing.T) {
	// Create a temporary file for testing
	tempDir, err := ioutil.TempDir("", "replication-state-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	seqMapPath := filepath.Join(tempDir, "seq-map.cbor")

	// Backup original path
	originalPath := cfg.Config.SeqMapPath
	defer func() {
		cfg.Config.SeqMapPath = originalPath
	}()

	// Set config to use the test file
	cfg.Config.SeqMapPath = seqMapPath

	t.Run("InitWithEmptyFile", func(t *testing.T) {
		// Create a new replicationState
		state := &replicationState{}

		// Initialize it with an empty file
		err := state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Check the initialized state
		assert.NotNil(t, state.seq)
		assert.NotNil(t, state.lock)
		assert.NotNil(t, state.fl)
		assert.Equal(t, 0, len(state.seq))
	})

	t.Run("InitWithExistingData", func(t *testing.T) {
		// Create test data
		testData := map[string]uint64{
			"stream1": 100,
			"stream2": 200,
		}

		// Write test data to the file
		file, err := os.Create(seqMapPath)
		assert.NoError(t, err)

		err = cbor.NewEncoder(file).Encode(testData)
		assert.NoError(t, err)
		file.Close()

		// Create a new replicationState
		state := &replicationState{}

		// Initialize it with the existing file
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Check that the data was loaded correctly
		assert.Equal(t, uint64(100), state.seq["stream1"])
		assert.Equal(t, uint64(200), state.seq["stream2"])
	})

	t.Run("InitWithCorruptedFile", func(t *testing.T) {
		// Write invalid CBOR data to the file
		err := ioutil.WriteFile(seqMapPath, []byte("not cbor data"), 0666)
		assert.NoError(t, err)

		// Create a new replicationState
		state := &replicationState{}

		// Initialize should return an error
		err = state.init()
		assert.Error(t, err)

		// Clean up if file was created despite error
		if state.fl != nil {
			state.fl.Close()
		}
	})
}

func TestReplicationState_Save(t *testing.T) {
	// Backup original path
	originalPath := cfg.Config.SeqMapPath
	defer func() {
		cfg.Config.SeqMapPath = originalPath
	}()

	t.Run("SaveWithUninitializedState", func(t *testing.T) {
		// Create an uninitialized state with a lock but no file
		state := &replicationState{
			seq:  make(map[string]uint64),
			lock: &sync.RWMutex{},
			fl:   nil, // Explicitly nil file
		}

		// Attempt to save without initializing
		_, err := state.save("stream1", 100)
		assert.Equal(t, ErrNotInitialized, err)
	})

	t.Run("SaveNewSequence", func(t *testing.T) {
		// Create a temporary file for this test
		tempDir, err := ioutil.TempDir("", "replication-state-test-save-new")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		seqMapPath := filepath.Join(tempDir, "seq-map.cbor")
		cfg.Config.SeqMapPath = seqMapPath

		// Create and initialize a state
		state := &replicationState{}
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Save a new sequence
		seq, err := state.save("stream1", 100)
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), seq)

		// Verify it was saved
		assert.Equal(t, uint64(100), state.seq["stream1"])

		// Verify it was written to the file by reading it back with a new state
		state.fl.Close()

		newState := &replicationState{}
		err = newState.init()
		assert.NoError(t, err)
		defer newState.fl.Close()

		assert.Equal(t, uint64(100), newState.seq["stream1"])
	})

	t.Run("SaveLowerSequence", func(t *testing.T) {
		// Create a temporary file for this test
		tempDir, err := ioutil.TempDir("", "replication-state-test-save-lower")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		seqMapPath := filepath.Join(tempDir, "seq-map.cbor")
		cfg.Config.SeqMapPath = seqMapPath

		// Create and initialize a state
		state := &replicationState{}
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Save an initial sequence
		_, err = state.save("stream1", 100)
		assert.NoError(t, err)

		// Try to save a lower sequence
		seq, err := state.save("stream1", 50)
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), seq) // Should return the existing higher value

		// Verify the value wasn't changed
		assert.Equal(t, uint64(100), state.seq["stream1"])
	})

	t.Run("SaveHigherSequence", func(t *testing.T) {
		// Create a temporary file for this test
		tempDir, err := ioutil.TempDir("", "replication-state-test-save-higher")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		seqMapPath := filepath.Join(tempDir, "seq-map.cbor")
		cfg.Config.SeqMapPath = seqMapPath

		// Create and initialize a state
		state := &replicationState{}
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Save an initial sequence
		_, err = state.save("stream1", 100)
		assert.NoError(t, err)

		// Save a higher sequence
		seq, err := state.save("stream1", 150)
		assert.NoError(t, err)
		assert.Equal(t, uint64(150), seq)

		// Verify it was updated
		assert.Equal(t, uint64(150), state.seq["stream1"])
	})

	t.Run("SaveMultipleStreams", func(t *testing.T) {
		// Create a temporary file for this test
		tempDir, err := ioutil.TempDir("", "replication-state-test-save-multiple")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		seqMapPath := filepath.Join(tempDir, "seq-map.cbor")
		cfg.Config.SeqMapPath = seqMapPath

		// Create and initialize a state
		state := &replicationState{}
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Save sequences for multiple streams
		_, err = state.save("stream1", 100)
		assert.NoError(t, err)

		_, err = state.save("stream2", 200)
		assert.NoError(t, err)

		// Verify both were saved
		assert.Equal(t, uint64(100), state.seq["stream1"])
		assert.Equal(t, uint64(200), state.seq["stream2"])

		// Verify they were written to the file
		state.fl.Close()

		newState := &replicationState{}
		err = newState.init()
		assert.NoError(t, err)
		defer newState.fl.Close()

		assert.Equal(t, uint64(100), newState.seq["stream1"])
		assert.Equal(t, uint64(200), newState.seq["stream2"])
	})
}

func TestReplicationState_Get(t *testing.T) {
	// Backup original path
	originalPath := cfg.Config.SeqMapPath
	defer func() {
		cfg.Config.SeqMapPath = originalPath
	}()

	t.Run("GetExistingSequence", func(t *testing.T) {
		// Create and initialize a state
		state := &replicationState{}
		state.seq = map[string]uint64{
			"stream1": 100,
			"stream2": 200,
		}
		state.lock = &sync.RWMutex{}

		// Get existing sequences
		assert.Equal(t, uint64(100), state.get("stream1"))
		assert.Equal(t, uint64(200), state.get("stream2"))
	})

	t.Run("GetNonExistingSequence", func(t *testing.T) {
		// Create and initialize a state
		state := &replicationState{}
		state.seq = map[string]uint64{
			"stream1": 100,
		}
		state.lock = &sync.RWMutex{}

		// Get a non-existing sequence
		assert.Equal(t, uint64(0), state.get("nonexistent"))
	})

	t.Run("GetWithRealFile", func(t *testing.T) {
		// Create a temporary file for this test
		tempDir, err := ioutil.TempDir("", "replication-state-test-get-real")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		seqMapPath := filepath.Join(tempDir, "seq-map.cbor")
		cfg.Config.SeqMapPath = seqMapPath

		// Create and initialize a state
		state := &replicationState{}
		err = state.init()
		assert.NoError(t, err)
		defer state.fl.Close()

		// Save some sequences
		_, err = state.save("stream1", 100)
		assert.NoError(t, err)

		_, err = state.save("stream2", 200)
		assert.NoError(t, err)

		// Get the sequences
		assert.Equal(t, uint64(100), state.get("stream1"))
		assert.Equal(t, uint64(200), state.get("stream2"))
		assert.Equal(t, uint64(0), state.get("nonexistent"))
	})
}
