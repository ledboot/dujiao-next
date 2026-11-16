package jsonmap

import (
	"reflect"
	"testing"
)

func TestJSONDatabaseRoundTrip(t *testing.T) {
	original := JSON{
		"enabled": true,
		"name":    "storefront",
		"nested":  map[string]interface{}{"count": float64(2)},
	}

	encoded, err := original.Value()
	if err != nil {
		t.Fatalf("encode JSON value: %v", err)
	}

	var decoded JSON
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("scan JSON value: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("JSON round trip mismatch\nwant: %#v\ngot:  %#v", original, decoded)
	}
}

func TestJSONScanNilCreatesEmptyObject(t *testing.T) {
	value := JSON{"stale": true}
	if err := value.Scan(nil); err != nil {
		t.Fatalf("scan nil JSON: %v", err)
	}
	if value == nil || len(value) != 0 {
		t.Fatalf("scan nil should create an empty object, got %#v", value)
	}
}

func TestJSONNilDatabaseValueStaysNull(t *testing.T) {
	var value JSON
	encoded, err := value.Value()
	if err != nil {
		t.Fatalf("encode nil JSON: %v", err)
	}
	if encoded != nil {
		t.Fatalf("nil JSON should produce a NULL driver value, got %#v", encoded)
	}
}

func TestJSONScanAcceptsTextStorage(t *testing.T) {
	// SQLite 导入/人工修复常把 JSON 列写成 TEXT，驱动返回 string。
	var value JSON
	if err := value.Scan(`{"gateway_url":"https://pay.example.com","enabled":true}`); err != nil {
		t.Fatalf("scan text JSON: %v", err)
	}
	if value["gateway_url"] != "https://pay.example.com" || value["enabled"] != true {
		t.Fatalf("unexpected decoded value: %#v", value)
	}
}

func TestJSONScanRejectsUnsupportedSource(t *testing.T) {
	var value JSON
	if err := value.Scan(123); err == nil {
		t.Fatal("expected error for unsupported scan source")
	}
}

func TestJSONScanRejectsInvalidJSON(t *testing.T) {
	var value JSON
	if err := value.Scan(`{"broken":`); err == nil {
		t.Fatal("expected error for invalid JSON text")
	}
}
