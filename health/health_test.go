package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	
	"github.com/wongfei2009/harmonylite/cfg"
)

// We need to create a mock that matches the HealthChecker interface

// MockStreamDB implements the minimal interface needed for testing
type MockStreamDB struct {
	connected bool
}

func (m *MockStreamDB) IsConnected() bool {
	return m.connected
}

func (m *MockStreamDB) AreCDCHooksInstalled() bool {
	return m.connected
}

func (m *MockStreamDB) GetTrackedTablesCount() int {
	return 3
}

func (m *MockStreamDB) DB() interface{} {
	return nil
}

// MockReplicator implements the minimal interface needed for testing
type MockReplicator struct {
	connected bool
}

func (m *MockReplicator) IsConnected() bool {
	return m.connected
}

func (m *MockReplicator) GetLastReplicatedEventTime() time.Time {
	return time.Now()
}

func (m *MockReplicator) GetLastPublishedEventTime() time.Time {
	return time.Now()
}

func TestHealthServer_HandleHealthCheck(t *testing.T) {
	// Test cases
	tests := []struct {
		name           string
		healthy        bool
		detailed       bool
		expectedStatus int
		checkBody      bool
	}{
		{
			name:           "Healthy_Detailed",
			healthy:        true,
			detailed:       true,
			expectedStatus: http.StatusOK,
			checkBody:      true,
		},
		{
			name:           "Unhealthy_Detailed",
			healthy:        false,
			detailed:       true,
			expectedStatus: http.StatusServiceUnavailable,
			checkBody:      true,
		},
		{
			name:           "Healthy_NotDetailed",
			healthy:        true,
			detailed:       false,
			expectedStatus: http.StatusOK,
			checkBody:      false,
		},
		{
			name:           "Unhealthy_NotDetailed",
			healthy:        false,
			detailed:       false,
			expectedStatus: http.StatusServiceUnavailable,
			checkBody:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create request
			req, err := http.NewRequest("GET", "/health", nil)
			if err != nil {
				t.Fatal(err)
			}

			// Create recorder
			rr := httptest.NewRecorder()

			// Create mock implementations
			mockDB := &MockStreamDB{
				connected: tc.healthy,
			}
			
			mockReplicator := &MockReplicator{
				connected: tc.healthy,
			}
			
			// Create real health checker with mocks
			checker := NewHealthChecker(mockDB, mockReplicator, 1, "test-version")

			config := &cfg.HealthCheckConfiguration{
				Enable:   true,
				Bind:     "0.0.0.0:8090",
				Path:     "/health",
				Detailed: tc.detailed,
			}

			server := NewHealthServer(config, checker)

			// Call handler directly
			handler := http.HandlerFunc(server.handleHealthCheck)
			handler.ServeHTTP(rr, req)

			// Check status code
			if status := rr.Code; status != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tc.expectedStatus)
			}

			// Check body if detailed is true
			if tc.checkBody {
				var status Status
				err = json.Unmarshal(rr.Body.Bytes(), &status)
				if err != nil {
					t.Errorf("Failed to parse response body: %v", err)
				}

				if tc.healthy && status.Status != "healthy" {
					t.Errorf("Expected status to be healthy, got %s", status.Status)
				}

				if !tc.healthy && status.Status != "unhealthy" {
					t.Errorf("Expected status to be unhealthy, got %s", status.Status)
				}

				if status.NodeID != 1 {
					t.Errorf("Expected nodeID to be 1, got %d", status.NodeID)
				}
			}
		})
	}
}

func TestHealthServer_StartStop(t *testing.T) {
	mockDB := &MockStreamDB{
		connected: true,
	}
	
	mockReplicator := &MockReplicator{
		connected: true,
	}
	
	checker := NewHealthChecker(mockDB, mockReplicator, 1, "test-version")

	config := &cfg.HealthCheckConfiguration{
		Enable:   true,
		Bind:     "127.0.0.1:0", // Use a random port to avoid conflicts
		Path:     "/health",
		Detailed: true,
	}

	server := NewHealthServer(config, checker)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	err = server.Stop()
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}
