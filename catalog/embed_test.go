package catalog

import (
	"testing"
)

func TestLookupAliases(t *testing.T) {
	// Execute tests validating the sync.Once embedded CSV parses natively populating targetIndex mapping accurately.

	tests := []struct {
		name        string
		query       string
		expectedID  string // The primary NGC ID we expect to get back
		expectError bool
	}{
		// 1. Primary ID Tests
		{name: "Exact Primary ID", query: "NGC1976", expectedID: "NGC1976"},
		{name: "Primary ID with Space", query: "NGC 1976", expectedID: "NGC1976"},
		
		// 2. Messier Alias Tests
		{name: "Messier Exact", query: "M42", expectedID: "NGC1976"},
		{name: "Messier with Space", query: "M 42", expectedID: "NGC1976"},
		{name: "Messier Lowercase", query: "m42", expectedID: "NGC1976"},
		{name: "Messier Hyphenated", query: "m-42", expectedID: "NGC1976"},
		
		// 3. Common Name Tests
		{name: "Common Name Exact", query: "Orion Nebula", expectedID: "NGC1976"},
		{name: "Common Name Lowercase", query: "orion nebula", expectedID: "NGC1976"},
		
		// 4. Identifiers Test (Assuming OpenNGC lists this identifier)
		{name: "Alternate Catalog", query: "LBN 974", expectedID: "NGC1976"},
		
		// 5. Failure State
		{name: "Unknown Target", query: "NotARealNebula", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := Lookup(tt.query)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected an error for query %q, got nil", tt.query)
				}
				return // Expected failure passed
			}

			if err != nil {
				t.Fatalf("unexpected error for query %q: %v", tt.query, err)
			}

			if target == nil {
				t.Fatalf("received nil target pointer for query %q", tt.query)
			}

			// Verify the alias successfully routed to the primary NGC object
			if target.ID != tt.expectedID {
				t.Errorf("query %q resolved to primary ID %q, expected %q", tt.query, target.ID, tt.expectedID)
			}
		})
	}
}
