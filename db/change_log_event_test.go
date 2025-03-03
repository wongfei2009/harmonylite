package db

import (
	"reflect"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/wongfei2009/harmonylite/core"
)

func TestChangeLogEvent_Wrap(t *testing.T) {
	// Create a test event with standard types
	event := ChangeLogEvent{
		Id:        123,
		Type:      "insert",
		TableName: "users",
		Row: map[string]any{
			"id":         1,
			"name":       "John Doe",
			"created_at": time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC),
			"active":     true,
			"score":      42.5,
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "created_at", Type: "TIMESTAMP", IsPrimaryKey: false},
			{Name: "active", Type: "BOOLEAN", IsPrimaryKey: false},
			{Name: "score", Type: "REAL", IsPrimaryKey: false},
		},
	}

	// Wrap the event
	wrapped, err := event.Wrap()
	if err != nil {
		t.Fatalf("Error wrapping event: %v", err)
	}

	// Check if the non-sensitive fields remain the same
	if wrapped.Id != event.Id {
		t.Errorf("Expected wrapped.Id = %d, got %d", event.Id, wrapped.Id)
	}
	if wrapped.Type != event.Type {
		t.Errorf("Expected wrapped.Type = %s, got %s", event.Type, wrapped.Type)
	}
	if wrapped.TableName != event.TableName {
		t.Errorf("Expected wrapped.TableName = %s, got %s", event.TableName, wrapped.TableName)
	}

	// Check if time fields are properly wrapped
	timeValue, ok := wrapped.Row["created_at"].(sensitiveTypeWrapper)
	if !ok {
		t.Errorf("Expected time value to be wrapped in sensitiveTypeWrapper")
	} else {
		originalTime := event.Row["created_at"].(time.Time)
		wrappedTime := timeValue.Time
		if !wrappedTime.Equal(originalTime) {
			t.Errorf("Expected wrapped time %v to equal original time %v", wrappedTime, originalTime)
		}
	}

	// Check if non-time fields are unchanged
	if wrapped.Row["id"] != event.Row["id"] {
		t.Errorf("Expected wrapped.Row[id] = %v, got %v", event.Row["id"], wrapped.Row["id"])
	}
	if wrapped.Row["name"] != event.Row["name"] {
		t.Errorf("Expected wrapped.Row[name] = %v, got %v", event.Row["name"], wrapped.Row["name"])
	}
	if wrapped.Row["active"] != event.Row["active"] {
		t.Errorf("Expected wrapped.Row[active] = %v, got %v", event.Row["active"], wrapped.Row["active"])
	}
	if wrapped.Row["score"] != event.Row["score"] {
		t.Errorf("Expected wrapped.Row[score] = %v, got %v", event.Row["score"], wrapped.Row["score"])
	}
}

func TestChangeLogEvent_Unwrap(t *testing.T) {
	// Create a wrapped test event
	originalTime := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	wrappedEvent := ChangeLogEvent{
		Id:        123,
		Type:      "insert",
		TableName: "users",
		Row: map[string]any{
			"id":         1,
			"name":       "John Doe",
			"created_at": sensitiveTypeWrapper{Time: &originalTime},
			"active":     true,
			"score":      42.5,
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "created_at", Type: "TIMESTAMP", IsPrimaryKey: false},
			{Name: "active", Type: "BOOLEAN", IsPrimaryKey: false},
			{Name: "score", Type: "REAL", IsPrimaryKey: false},
		},
	}

	// Unwrap the event
	unwrapped, err := wrappedEvent.Unwrap()
	if err != nil {
		t.Fatalf("Error unwrapping event: %v", err)
	}

	// Check if the non-sensitive fields remain the same
	if unwrapped.Id != wrappedEvent.Id {
		t.Errorf("Expected unwrapped.Id = %d, got %d", wrappedEvent.Id, unwrapped.Id)
	}
	if unwrapped.Type != wrappedEvent.Type {
		t.Errorf("Expected unwrapped.Type = %s, got %s", wrappedEvent.Type, unwrapped.Type)
	}
	if unwrapped.TableName != wrappedEvent.TableName {
		t.Errorf("Expected unwrapped.TableName = %s, got %s", wrappedEvent.TableName, unwrapped.TableName)
	}

	// Check if time fields are properly unwrapped
	switch unwrappedTime := unwrapped.Row["created_at"].(type) {
	case time.Time:
		if !unwrappedTime.Equal(originalTime) {
			t.Errorf("Expected unwrapped time %v to equal original time %v", unwrappedTime, originalTime)
		}
	case *time.Time:
		if !unwrappedTime.Equal(originalTime) {
			t.Errorf("Expected unwrapped time %v to equal original time %v", unwrappedTime, originalTime)
		}
	default:
		t.Errorf("Expected unwrapped time value to be time.Time or *time.Time, got %T", unwrapped.Row["created_at"])
	}

	// Check if non-time fields are unchanged
	if unwrapped.Row["id"] != wrappedEvent.Row["id"] {
		t.Errorf("Expected unwrapped.Row[id] = %v, got %v", wrappedEvent.Row["id"], unwrapped.Row["id"])
	}
	if unwrapped.Row["name"] != wrappedEvent.Row["name"] {
		t.Errorf("Expected unwrapped.Row[name] = %v, got %v", wrappedEvent.Row["name"], unwrapped.Row["name"])
	}
	if unwrapped.Row["active"] != wrappedEvent.Row["active"] {
		t.Errorf("Expected unwrapped.Row[active] = %v, got %v", wrappedEvent.Row["active"], unwrapped.Row["active"])
	}
	if unwrapped.Row["score"] != wrappedEvent.Row["score"] {
		t.Errorf("Expected unwrapped.Row[score] = %v, got %v", wrappedEvent.Row["score"], unwrapped.Row["score"])
	}
}

func TestChangeLogEvent_Hash(t *testing.T) {
	// Create events with the same data but different non-hash-relevant fields
	event1 := ChangeLogEvent{
		Id:        123,
		Type:      "insert",
		TableName: "users",
		Row: map[string]any{
			"id":    1,
			"name":  "John Doe",
			"email": "john@example.com",
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "email", Type: "TEXT", IsPrimaryKey: false},
		},
	}

	event2 := ChangeLogEvent{
		Id:        456,      // Different ID should not affect hash
		Type:      "update", // Different type should not affect hash
		TableName: "users",
		Row: map[string]any{
			"id":    1,                       // Same primary key
			"name":  "Different Name",        // Different non-PK value should not affect hash
			"email": "different@example.com", // Different non-PK value should not affect hash
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "email", Type: "TEXT", IsPrimaryKey: false},
		},
	}

	// Create an event with a different hash-relevant field
	event3 := ChangeLogEvent{
		Id:        789,
		Type:      "insert",
		TableName: "users",
		Row: map[string]any{
			"id":    2, // Different primary key should cause different hash
			"name":  "Jane Doe",
			"email": "jane@example.com",
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "email", Type: "TEXT", IsPrimaryKey: false},
		},
	}

	// Test with a different table name
	event4 := ChangeLogEvent{
		Id:        123,
		Type:      "insert",
		TableName: "profiles", // Different table name should cause different hash
		Row: map[string]any{
			"id":    1,
			"name":  "John Doe",
			"email": "john@example.com",
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "email", Type: "TEXT", IsPrimaryKey: false},
		},
	}

	// Calculate hashes
	hash1, err := event1.Hash()
	if err != nil {
		t.Fatalf("Error calculating hash for event1: %v", err)
	}

	hash2, err := event2.Hash()
	if err != nil {
		t.Fatalf("Error calculating hash for event2: %v", err)
	}

	hash3, err := event3.Hash()
	if err != nil {
		t.Fatalf("Error calculating hash for event3: %v", err)
	}

	hash4, err := event4.Hash()
	if err != nil {
		t.Fatalf("Error calculating hash for event4: %v", err)
	}

	// Verify hashes
	if hash1 != hash2 {
		t.Errorf("Expected hash1 == hash2, got %d != %d", hash1, hash2)
	}

	if hash1 == hash3 {
		t.Errorf("Expected hash1 != hash3, got %d == %d", hash1, hash3)
	}

	if hash1 == hash4 {
		t.Errorf("Expected hash1 != hash4, got %d == %d", hash1, hash4)
	}
}

func TestChangeLogEvent_MarshalUnmarshal(t *testing.T) {
	// Create an event with a time field
	originalTime := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	event := ChangeLogEvent{
		Id:        123,
		Type:      "insert",
		TableName: "users",
		Row: map[string]any{
			"id":         1,
			"name":       "John Doe",
			"created_at": originalTime,
		},
		tableInfo: []*ColumnInfo{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true},
			{Name: "name", Type: "TEXT", IsPrimaryKey: false},
			{Name: "created_at", Type: "TIMESTAMP", IsPrimaryKey: false},
		},
	}

	// Wrap and marshal the event
	wrapped, err := event.Wrap()
	if err != nil {
		t.Fatalf("Error wrapping event: %v", err)
	}

	em, err := cbor.EncOptions{}.EncModeWithTags(core.CBORTags)
	if err != nil {
		t.Fatalf("Error creating encoder: %v", err)
	}

	data, err := em.Marshal(wrapped)
	if err != nil {
		t.Fatalf("Error marshaling event: %v", err)
	}

	// Unmarshal back to an event
	var unmarshaled ChangeLogEvent
	dm, err := cbor.DecOptions{}.DecModeWithTags(core.CBORTags)
	if err != nil {
		t.Fatalf("Error creating decoder: %v", err)
	}

	err = dm.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Error unmarshaling event: %v", err)
	}

	// Verify the unmarshaled event
	if unmarshaled.Id != event.Id {
		t.Errorf("Expected unmarshaled.Id = %d, got %d", event.Id, unmarshaled.Id)
	}
	if unmarshaled.Type != event.Type {
		t.Errorf("Expected unmarshaled.Type = %s, got %s", event.Type, unmarshaled.Type)
	}
	if unmarshaled.TableName != event.TableName {
		t.Errorf("Expected unmarshaled.TableName = %s, got %s", event.TableName, unmarshaled.TableName)
	}

	// Check if time field was correctly marshaled and unmarshaled
	timeWrapper, ok := unmarshaled.Row["created_at"].(sensitiveTypeWrapper)
	if !ok {
		t.Errorf("Expected unmarshaled time to be a sensitiveTypeWrapper, got %T", unmarshaled.Row["created_at"])
	} else {
		if !timeWrapper.Time.Equal(originalTime) {
			t.Errorf("Expected unmarshaled time %v to equal original time %v", timeWrapper.Time, originalTime)
		}
	}

	// Unwrap the unmarshaled event
	unwrapped, err := unmarshaled.Unwrap()
	if err != nil {
		t.Fatalf("Error unwrapping unmarshaled event: %v", err)
	}

	// Check the unwrapped time
	switch unwrappedTime := unwrapped.Row["created_at"].(type) {
	case time.Time:
		if !unwrappedTime.Equal(originalTime) {
			t.Errorf("Expected unwrapped time %v to equal original time %v", unwrappedTime, originalTime)
		}
	case *time.Time:
		if !unwrappedTime.Equal(originalTime) {
			t.Errorf("Expected unwrapped time %v to equal original time %v", unwrappedTime, originalTime)
		}
	default:
		t.Errorf("Expected unwrapped time to be a time.Time or *time.Time, got %T", unwrapped.Row["created_at"])
	}
}

func TestSensitiveTypeWrapper_GetValue(t *testing.T) {
	// Test with time
	now := time.Now()
	wrapper := sensitiveTypeWrapper{
		Time: &now,
	}

	val := wrapper.GetValue()

	// The return type could be either time.Time or *time.Time depending on implementation
	switch timeVal := val.(type) {
	case time.Time:
		if !timeVal.Equal(now) {
			t.Errorf("Expected GetValue() to return %v, got %v", now, timeVal)
		}
	case *time.Time:
		if !timeVal.Equal(now) {
			t.Errorf("Expected GetValue() to return %v, got %v", now, timeVal)
		}
	default:
		t.Errorf("Expected GetValue() to return time.Time or *time.Time, got %T", val)
	}

	// Test with nil
	wrapper = sensitiveTypeWrapper{
		Time: nil,
	}

	val = wrapper.GetValue()
	// Don't directly compare with nil as the actual value might be
	// a typed nil which doesn't equal untyped nil in Go
	if val != nil && val != (*time.Time)(nil) {
		t.Errorf("Expected GetValue() to return nil or nil pointer, got %v (type: %T)", val, val)
	}
}

func TestChangeLogEvent_getSortedPKColumns(t *testing.T) {
	// Create an event with multiple primary keys in unsorted order
	event := ChangeLogEvent{
		TableName: "test_table",
		tableInfo: []*ColumnInfo{
			{Name: "c", Type: "INTEGER", IsPrimaryKey: true, PrimaryKeyIndex: 3},
			{Name: "a", Type: "TEXT", IsPrimaryKey: true, PrimaryKeyIndex: 1},
			{Name: "d", Type: "INTEGER", IsPrimaryKey: false},
			{Name: "b", Type: "TEXT", IsPrimaryKey: true, PrimaryKeyIndex: 2},
		},
	}

	// Get sorted primary key columns
	columns := event.getSortedPKColumns()

	// Expected result: [a, b, c] (alphabetically sorted)
	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(columns, expected) {
		t.Errorf("Expected getSortedPKColumns() to return %v, got %v", expected, columns)
	}

	// Test cache by modifying the table info (should still use cached result)
	event.tableInfo = []*ColumnInfo{
		{Name: "z", Type: "INTEGER", IsPrimaryKey: true},
		{Name: "y", Type: "TEXT", IsPrimaryKey: true},
	}

	columns = event.getSortedPKColumns()
	if !reflect.DeepEqual(columns, expected) {
		t.Errorf("Expected getSortedPKColumns() to return cached result %v, got %v", expected, columns)
	}

	// Clear cache and test again
	tablePKColumnsLock.Lock()
	delete(tablePKColumnsCache, event.TableName)
	tablePKColumnsLock.Unlock()

	columns = event.getSortedPKColumns()
	expectedAfterCacheClear := []string{"y", "z"}
	if !reflect.DeepEqual(columns, expectedAfterCacheClear) {
		t.Errorf("Expected getSortedPKColumns() to return %v after cache clear, got %v", expectedAfterCacheClear, columns)
	}
}
