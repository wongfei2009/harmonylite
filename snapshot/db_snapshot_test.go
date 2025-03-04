package snapshot

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Create interfaces that match the subset of methods we need to mock
type dbBackupper interface {
	BackupTo(path string) error
	GetPath() string
}

// MockDB implements the dbBackupper interface
type MockDB struct {
	mock.Mock
}

func (m *MockDB) BackupTo(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockDB) GetPath() string {
	args := m.Called()
	return args.String(0)
}

// MockStorage implements the Storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Upload(name, filePath string) error {
	args := m.Called(name, filePath)
	return args.Error(0)
}

func (m *MockStorage) Download(filePath, name string) error {
	args := m.Called(filePath, name)
	return args.Error(0)
}

// Create a test version of NatsDBSnapshot that uses our interface
type testNatsDBSnapshot struct {
	mutex   *sync.Mutex
	db      dbBackupper
	storage Storage
}

// Copy of the main NewNatsDBSnapshot constructor but using our interface
func newTestSnapshot(d dbBackupper, s Storage) *testNatsDBSnapshot {
	return &testNatsDBSnapshot{
		mutex:   &sync.Mutex{},
		db:      d,
		storage: s,
	}
}

// Copy of the SaveSnapshot method but adapted for our test struct
func (n *testNatsDBSnapshot) SaveSnapshot() error {
	locked := n.mutex.TryLock()
	if !locked {
		return ErrPendingSnapshot
	}

	defer n.mutex.Unlock()
	tmpSnapshot, err := os.MkdirTemp(os.TempDir(), tempDirPattern)
	if err != nil {
		return err
	}
	defer cleanupDir(tmpSnapshot)

	bkFilePath := path.Join(tmpSnapshot, snapshotFileName)
	err = n.db.BackupTo(bkFilePath)
	if err != nil {
		return err
	}

	return n.storage.Upload(snapshotFileName, bkFilePath)
}

// Copy of the RestoreSnapshot method but adapted for our test struct
// and using a function parameter for RestoreFrom to make it testable
func (n *testNatsDBSnapshot) RestoreSnapshot(restoreFromFn func(destPath, bkFilePath string) error) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	tmpSnapshotPath, err := os.MkdirTemp(os.TempDir(), tempDirPattern)
	if err != nil {
		return err
	}
	defer cleanupDir(tmpSnapshotPath)

	bkFilePath := path.Join(tmpSnapshotPath, snapshotFileName)
	err = n.storage.Download(bkFilePath, snapshotFileName)
	if err == ErrNoSnapshotFound {
		return nil
	}

	if err != nil {
		return err
	}

	err = restoreFromFn(n.db.GetPath(), bkFilePath)
	if err != nil {
		return err
	}

	return nil
}

// TestNatsDBSnapshot_SaveSnapshot tests the SaveSnapshot method
func TestNatsDBSnapshot_SaveSnapshot(t *testing.T) {
	t.Run("SuccessfulSave", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Set expectations
		mockDB.On("BackupTo", mock.AnythingOfType("string")).Return(nil)
		mockStorage.On("Upload", snapshotFileName, mock.AnythingOfType("string")).Return(nil)

		// Call the method
		err := snapshot.SaveSnapshot()

		// Assert
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockStorage.AssertExpectations(t)
	})

	t.Run("ConcurrentSaveAttempts", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object with a manually controlled mutex
		snapshot := &testNatsDBSnapshot{
			mutex:   &sync.Mutex{},
			db:      mockDB,
			storage: mockStorage,
		}

		// Lock the mutex to simulate an ongoing snapshot
		snapshot.mutex.Lock()

		// Try to save while locked
		err := snapshot.SaveSnapshot()

		// Assert that we get the pending error
		assert.Equal(t, ErrPendingSnapshot, err)

		// Unlock for cleanup
		snapshot.mutex.Unlock()
	})

	t.Run("BackupFailure", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Expected error
		expectedErr := errors.New("backup failed")

		// Set expectations
		mockDB.On("BackupTo", mock.AnythingOfType("string")).Return(expectedErr)

		// Call the method
		err := snapshot.SaveSnapshot()

		// Assert
		assert.Equal(t, expectedErr, err)
		mockDB.AssertExpectations(t)
		// Storage should not be called if backup fails
		mockStorage.AssertNotCalled(t, "Upload")
	})

	t.Run("UploadFailure", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Expected error
		expectedErr := errors.New("upload failed")

		// Set expectations
		mockDB.On("BackupTo", mock.AnythingOfType("string")).Return(nil)
		mockStorage.On("Upload", snapshotFileName, mock.AnythingOfType("string")).Return(expectedErr)

		// Call the method
		err := snapshot.SaveSnapshot()

		// Assert
		assert.Equal(t, expectedErr, err)
		mockDB.AssertExpectations(t)
		mockStorage.AssertExpectations(t)
	})
}

// TestNatsDBSnapshot_RestoreSnapshot tests the RestoreSnapshot method
func TestNatsDBSnapshot_RestoreSnapshot(t *testing.T) {
	t.Run("SuccessfulRestore", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Set up expectations
		dbPath := "/path/to/db.sqlite"
		mockDB.On("GetPath").Return(dbPath)
		mockStorage.On("Download", mock.AnythingOfType("string"), snapshotFileName).Return(nil)

		// Mock restore function
		restoreCalled := false
		mockRestoreFrom := func(destPath, bkFilePath string) error {
			assert.Equal(t, dbPath, destPath)
			assert.Contains(t, bkFilePath, snapshotFileName)
			restoreCalled = true
			return nil
		}

		// Call the method
		err := snapshot.RestoreSnapshot(mockRestoreFrom)

		// Assert
		assert.NoError(t, err)
		assert.True(t, restoreCalled, "RestoreFrom should have been called")
		mockDB.AssertExpectations(t)
		mockStorage.AssertExpectations(t)
	})

	t.Run("NoSnapshotFound", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Set up expectations
		mockStorage.On("Download", mock.AnythingOfType("string"), snapshotFileName).Return(ErrNoSnapshotFound)

		// Call the method
		mockRestoreFrom := func(destPath, bkFilePath string) error {
			t.Fatal("RestoreFrom should not be called")
			return nil
		}

		err := snapshot.RestoreSnapshot(mockRestoreFrom)

		// Assert
		assert.NoError(t, err, "Should not return error when no snapshot is found")
		mockStorage.AssertExpectations(t)
		mockDB.AssertNotCalled(t, "GetPath")
	})

	t.Run("DownloadFailure", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Expected error
		expectedErr := errors.New("download failed")

		// Set up expectations
		mockStorage.On("Download", mock.AnythingOfType("string"), snapshotFileName).Return(expectedErr)

		// Call the method
		mockRestoreFrom := func(destPath, bkFilePath string) error {
			t.Fatal("RestoreFrom should not be called")
			return nil
		}

		err := snapshot.RestoreSnapshot(mockRestoreFrom)

		// Assert
		assert.Equal(t, expectedErr, err)
		mockStorage.AssertExpectations(t)
		mockDB.AssertNotCalled(t, "GetPath")
	})

	t.Run("RestoreFailure", func(t *testing.T) {
		// Create mocks
		mockDB := new(MockDB)
		mockStorage := new(MockStorage)

		// Create snapshot object
		snapshot := newTestSnapshot(mockDB, mockStorage)

		// Expected error
		expectedErr := errors.New("restore failed")

		// Set up expectations
		dbPath := "/path/to/db.sqlite"
		mockDB.On("GetPath").Return(dbPath)
		mockStorage.On("Download", mock.AnythingOfType("string"), snapshotFileName).Return(nil)

		// Mock restore function that fails
		mockRestoreFrom := func(destPath, bkFilePath string) error {
			return expectedErr
		}

		// Call the method
		err := snapshot.RestoreSnapshot(mockRestoreFrom)

		// Assert
		assert.Equal(t, expectedErr, err)
		mockDB.AssertExpectations(t)
		mockStorage.AssertExpectations(t)
	})
}

// TestFileHash tests the fileHash function
func TestFileHash(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "hash-test-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write some content
	content := "This is test content for hashing"
	_, err = tmpFile.Write([]byte(content))
	assert.NoError(t, err)
	tmpFile.Close()

	// Get the hash
	hash1, err := fileHash(tmpFile.Name())
	assert.NoError(t, err)
	assert.NotEmpty(t, hash1)

	// Get the hash again to verify it's deterministic
	hash2, err := fileHash(tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Test with a non-existent file
	_, err = fileHash("non-existent-file")
	assert.Error(t, err)
}

// TestCleanupDir tests the cleanupDir function
func TestCleanupDir(t *testing.T) {
	t.Run("SuccessfulCleanup", func(t *testing.T) {
		// Create a temporary directory
		tmpDir, err := os.MkdirTemp("", tempDirPattern)
		assert.NoError(t, err)

		// Create a file in the directory
		testFile := path.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0644)
		assert.NoError(t, err)

		// Call cleanupDir
		cleanupDir(tmpDir)

		// Verify directory is gone
		_, err = os.Stat(tmpDir)
		assert.True(t, os.IsNotExist(err), "Directory should not exist after cleanup")
	})

	t.Run("CleanupNonexistentDir", func(t *testing.T) {
		// Use a path that doesn't exist
		nonExistentDir := path.Join(os.TempDir(), "nonexistent-"+fmt.Sprintf("%d", time.Now().UnixNano()))

		// Call cleanupDir - this should not panic
		cleanupDir(nonExistentDir)
	})
}
