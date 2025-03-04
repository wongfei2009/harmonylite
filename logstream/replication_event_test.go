package logstream

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/wongfei2009/harmonylite/core"
)

// MockEvent implements the core.ReplicableEvent interface for testing
type MockEvent struct {
	ID        int64
	Name      string
	Timestamp time.Time
	Data      map[string]interface{}
	Wrapped   bool // Flag to track if the event has been wrapped/unwrapped
}

// Wrap satisfies the core.ReplicableEvent interface
func (m MockEvent) Wrap() (MockEvent, error) {
	// Create a copy with the wrapped flag set to true
	return MockEvent{
		ID:        m.ID,
		Name:      m.Name,
		Timestamp: m.Timestamp,
		Data:      m.Data,
		Wrapped:   true,
	}, nil
}

// Unwrap satisfies the core.ReplicableEvent interface
func (m MockEvent) Unwrap() (MockEvent, error) {
	// Create a copy with the wrapped flag set to false
	return MockEvent{
		ID:        m.ID,
		Name:      m.Name,
		Timestamp: m.Timestamp,
		Data:      m.Data,
		Wrapped:   false,
	}, nil
}

// timestampsApproximatelyEqual checks if two timestamps are equal within a small tolerance
// This accounts for precision loss during serialization
func timestampsApproximatelyEqual(t1, t2 time.Time) bool {
	// If either time is the zero value, they should match exactly
	if t1.IsZero() || t2.IsZero() {
		return t1.Equal(t2)
	}

	// Compare only to second precision
	return t1.Truncate(time.Second).Equal(t2.Truncate(time.Second))
}

// Register the MockEvent type with CBOR before running tests
func init() {
	// Register the MockEvent type with CBOR
	err := core.CBORTags.Add(
		cbor.TagOptions{
			DecTag: cbor.DecTagOptional,
			EncTag: cbor.EncTagRequired,
		},
		reflect.TypeOf(MockEvent{}),
		1000, // Use a test tag number
	)

	if err != nil {
		panic(err)
	}
}

func TestReplicationEvent_Marshal(t *testing.T) {
	// Create a fixed timestamp without microseconds for consistent testing
	fixedTime := time.Date(2025, 3, 4, 16, 22, 22, 0, time.Local) // No microseconds

	// Test cases
	tests := []struct {
		name    string
		event   ReplicationEvent[MockEvent]
		wantErr bool
	}{
		{
			name: "Basic event",
			event: ReplicationEvent[MockEvent]{
				FromNodeId: 123,
				Payload: MockEvent{
					ID:        1,
					Name:      "Test Event",
					Timestamp: time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC),
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": 42,
					},
					Wrapped: false,
				},
			},
			wantErr: false,
		},
		{
			name: "Event with empty payload",
			event: ReplicationEvent[MockEvent]{
				FromNodeId: 456,
				Payload: MockEvent{
					ID:        2,
					Name:      "",
					Timestamp: time.Time{},
					Data:      map[string]interface{}{},
					Wrapped:   false,
				},
			},
			wantErr: false,
		},
		{
			name: "Event with complex data",
			event: ReplicationEvent[MockEvent]{
				FromNodeId: 789,
				Payload: MockEvent{
					ID:        3,
					Name:      "Complex Event",
					Timestamp: fixedTime, // Using fixed timestamp without microseconds
					Data: map[string]interface{}{
						"nestedMap": map[string]interface{}{
							"nested1": "value",
							"nested2": 123,
						},
						"arrayValue": []int{1, 2, 3, 4, 5},
					},
					Wrapped: false,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the event
			data, err := tt.event.Marshal()

			// Check error condition matches expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplicationEvent.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify data is not empty
			if len(data) == 0 {
				t.Errorf("ReplicationEvent.Marshal() returned empty data")
				return
			}

			// Unmarshal back and verify
			var unmarshaledEvent ReplicationEvent[MockEvent]
			err = unmarshaledEvent.Unmarshal(data)
			if err != nil {
				t.Errorf("Failed to unmarshal marshaled data: %v", err)
				return
			}

			// Check that node ID was preserved
			if unmarshaledEvent.FromNodeId != tt.event.FromNodeId {
				t.Errorf("Unmarshaled event has different FromNodeId: got %v, want %v",
					unmarshaledEvent.FromNodeId, tt.event.FromNodeId)
			}

			// Check basic payload fields
			if unmarshaledEvent.Payload.ID != tt.event.Payload.ID {
				t.Errorf("Unmarshaled payload has different ID: got %v, want %v",
					unmarshaledEvent.Payload.ID, tt.event.Payload.ID)
			}

			if unmarshaledEvent.Payload.Name != tt.event.Payload.Name {
				t.Errorf("Unmarshaled payload has different Name: got %v, want %v",
					unmarshaledEvent.Payload.Name, tt.event.Payload.Name)
			}

			// Use the more robust timestamp comparison with detailed diagnostics
			if !timestampsApproximatelyEqual(unmarshaledEvent.Payload.Timestamp, tt.event.Payload.Timestamp) {
				t.Errorf("Unmarshaled payload has different Timestamp: got %v (Unix: %d.%09d), want %v (Unix: %d.%09d)",
					unmarshaledEvent.Payload.Timestamp,
					unmarshaledEvent.Payload.Timestamp.Unix(),
					unmarshaledEvent.Payload.Timestamp.Nanosecond(),
					tt.event.Payload.Timestamp,
					tt.event.Payload.Timestamp.Unix(),
					tt.event.Payload.Timestamp.Nanosecond())
			}

			// Verify the wrap/unwrap process was applied
			if unmarshaledEvent.Payload.Wrapped != false {
				t.Errorf("Payload should be unwrapped after unmarshaling, but Wrapped = %v",
					unmarshaledEvent.Payload.Wrapped)
			}
		})
	}
}

func TestReplicationEvent_Unmarshal(t *testing.T) {
	// Create a sample event
	originalEvent := ReplicationEvent[MockEvent]{
		FromNodeId: 123,
		Payload: MockEvent{
			ID:        42,
			Name:      "Test Event",
			Timestamp: time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC),
			Data: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			Wrapped: false,
		},
	}

	// Marshal it
	data, err := originalEvent.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal event for unmarshal test: %v", err)
	}

	// Test cases for unmarshaling
	tests := []struct {
		name        string
		data        []byte
		wantErr     bool
		corruptData func([]byte) []byte // Function to corrupt data for negative tests
	}{
		{
			name:    "Valid data",
			data:    data,
			wantErr: false,
		},
		{
			name:    "Empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "Corrupted data",
			data:    data,
			wantErr: true,
			corruptData: func(data []byte) []byte {
				// Just corrupt some bytes in the middle
				if len(data) > 10 {
					corrupted := make([]byte, len(data))
					copy(corrupted, data)
					corrupted[len(data)/2] = 0xFF
					corrupted[len(data)/2+1] = 0xFF
					return corrupted
				}
				return data
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testData := tt.data
			if tt.corruptData != nil {
				testData = tt.corruptData(testData)
			}

			var unmarshaledEvent ReplicationEvent[MockEvent]
			err := unmarshaledEvent.Unmarshal(testData)

			// Check error condition matches expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplicationEvent.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check that unmarshaled event matches original event
			if unmarshaledEvent.FromNodeId != originalEvent.FromNodeId {
				t.Errorf("Unmarshaled event has different FromNodeId: got %v, want %v",
					unmarshaledEvent.FromNodeId, originalEvent.FromNodeId)
			}

			if unmarshaledEvent.Payload.ID != originalEvent.Payload.ID {
				t.Errorf("Unmarshaled payload has different ID: got %v, want %v",
					unmarshaledEvent.Payload.ID, originalEvent.Payload.ID)
			}

			if unmarshaledEvent.Payload.Name != originalEvent.Payload.Name {
				t.Errorf("Unmarshaled payload has different Name: got %v, want %v",
					unmarshaledEvent.Payload.Name, originalEvent.Payload.Name)
			}

			// Use the robust timestamp comparison
			if !timestampsApproximatelyEqual(unmarshaledEvent.Payload.Timestamp, originalEvent.Payload.Timestamp) {
				t.Errorf("Unmarshaled payload has different Timestamp: got %v (Unix: %d.%09d), want %v (Unix: %d.%09d)",
					unmarshaledEvent.Payload.Timestamp,
					unmarshaledEvent.Payload.Timestamp.Unix(),
					unmarshaledEvent.Payload.Timestamp.Nanosecond(),
					originalEvent.Payload.Timestamp,
					originalEvent.Payload.Timestamp.Unix(),
					originalEvent.Payload.Timestamp.Nanosecond())
			}
		})
	}
}

func TestReplicationEvent_RoundTrip(t *testing.T) {
	// Create fixed timestamps without microseconds for consistent testing
	fixedTime1 := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	fixedTime2 := time.Date(2025, 3, 2, 11, 30, 0, 0, time.UTC)
	fixedTime3 := time.Date(2025, 3, 3, 15, 45, 0, 0, time.UTC)

	// Test a variety of payload sizes and contents
	testCases := []struct {
		name    string
		payload MockEvent
	}{
		{
			name: "Small payload",
			payload: MockEvent{
				ID:        1,
				Name:      "Small",
				Timestamp: fixedTime1,
				Data: map[string]interface{}{
					"key": "value",
				},
			},
		},
		{
			name: "Medium payload",
			payload: MockEvent{
				ID:        2,
				Name:      "Medium",
				Timestamp: fixedTime2,
				Data: map[string]interface{}{
					"key1": "value1",
					"key2": 42,
					"key3": true,
					"key4": []string{"a", "b", "c"},
				},
			},
		},
		{
			name: "Large payload",
			payload: MockEvent{
				ID:        3,
				Name:      "Large",
				Timestamp: fixedTime3,
				Data:      generateLargeData(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the event
			original := ReplicationEvent[MockEvent]{
				FromNodeId: 42,
				Payload:    tc.payload,
			}

			// Marshal
			data, err := original.Marshal()
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal
			var unmarshaled ReplicationEvent[MockEvent]
			err = unmarshaled.Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Check the node ID
			if unmarshaled.FromNodeId != original.FromNodeId {
				t.Errorf("Node ID mismatch: got %d, want %d",
					unmarshaled.FromNodeId, original.FromNodeId)
			}

			// Check basic payload fields
			if unmarshaled.Payload.ID != original.Payload.ID {
				t.Errorf("Payload ID mismatch: got %d, want %d",
					unmarshaled.Payload.ID, original.Payload.ID)
			}

			if unmarshaled.Payload.Name != original.Payload.Name {
				t.Errorf("Payload Name mismatch: got %s, want %s",
					unmarshaled.Payload.Name, original.Payload.Name)
			}

			// Use the more robust timestamp comparison
			if !timestampsApproximatelyEqual(unmarshaled.Payload.Timestamp, original.Payload.Timestamp) {
				t.Errorf("Payload Timestamp mismatch: got %v (Unix: %d.%09d), want %v (Unix: %d.%09d)",
					unmarshaled.Payload.Timestamp,
					unmarshaled.Payload.Timestamp.Unix(),
					unmarshaled.Payload.Timestamp.Nanosecond(),
					original.Payload.Timestamp,
					original.Payload.Timestamp.Unix(),
					original.Payload.Timestamp.Nanosecond())
			}

			// Verify the wrap/unwrap flag
			if unmarshaled.Payload.Wrapped != false {
				t.Errorf("Expected Payload.Wrapped to be false, got true")
			}

			// Check data map keys (complete equality might be tricky with maps)
			for key := range original.Payload.Data {
				if _, exists := unmarshaled.Payload.Data[key]; !exists {
					t.Errorf("Key %s missing from unmarshaled data", key)
				}
			}
		})
	}
}

// generateLargeData creates a larger, more complex data structure for testing
func generateLargeData() map[string]interface{} {
	data := make(map[string]interface{})

	// Add some basic key-values
	for i := 0; i < 50; i++ {
		data[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
	}

	// Add nested maps
	nestedMap := make(map[string]interface{})
	for i := 0; i < 20; i++ {
		nestedMap[fmt.Sprintf("nested%d", i)] = i
	}
	data["nested"] = nestedMap

	// Add array values
	data["array"] = make([]int, 100)
	for i := 0; i < 100; i++ {
		data["array"].([]int)[i] = i
	}

	return data
}

// TestMarshalPerformance measures the performance of marshaling and unmarshaling
// This test is skipped by default but can be run with -test.run=Performance
func TestMarshalPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Use a fixed timestamp without microseconds for consistency
	fixedTime := time.Date(2025, 3, 4, 16, 22, 22, 0, time.UTC)

	payload := MockEvent{
		ID:        1,
		Name:      "Performance Test",
		Timestamp: fixedTime,
		Data:      generateLargeData(),
	}

	event := ReplicationEvent[MockEvent]{
		FromNodeId: 42,
		Payload:    payload,
	}

	// Benchmark marshaling
	t.Run("Marshal", func(t *testing.T) {
		start := time.Now()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			_, err := event.Marshal()
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
		}

		elapsed := time.Since(start)
		t.Logf("Marshal: %d iterations in %s (%.2f µs/op)",
			iterations, elapsed, float64(elapsed.Microseconds())/float64(iterations))
	})

	// Create marshaled data for unmarshal test
	data, err := event.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal for benchmark: %v", err)
	}

	// Benchmark unmarshaling
	t.Run("Unmarshal", func(t *testing.T) {
		start := time.Now()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			var unmarshaled ReplicationEvent[MockEvent]
			err := unmarshaled.Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
		}

		elapsed := time.Since(start)
		t.Logf("Unmarshal: %d iterations in %s (%.2f µs/op)",
			iterations, elapsed, float64(elapsed.Microseconds())/float64(iterations))
	})
}
