package config

import (
	"reflect"
	"testing"
)

func TestSecretRefStructure(t *testing.T) {
	// Reflect test: SecretRef exposes only Source field (no value-bearing field).
	rt := reflect.TypeOf(SecretRef{})

	if rt.Kind() != reflect.Struct {
		t.Fatalf("SecretRef must be a struct, got %v", rt.Kind())
	}

	// Count exported fields
	if rt.NumField() != 1 {
		t.Fatalf("SecretRef must have exactly 1 field, got %d", rt.NumField())
	}

	field := rt.Field(0)
	if field.Name != "Source" {
		t.Errorf("SecretRef's only field must be 'Source', got %q", field.Name)
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("SecretRef.Source must be a string, got %v", field.Type.Kind())
	}
}
