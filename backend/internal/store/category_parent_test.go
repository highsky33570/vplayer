package store

import (
	"database/sql"
	"testing"
)

func TestNullUint64Ptr(t *testing.T) {
	if got := nullUint64Ptr(sql.NullInt64{}); got != nil {
		t.Fatalf("invalid NullInt64 => nil, got %v", got)
	}
	got := nullUint64Ptr(sql.NullInt64{Int64: 42, Valid: true})
	if got == nil || *got != 42 {
		t.Fatalf("valid 42 => *42, got %v", got)
	}
}

func TestParentIDSQLAndEqual(t *testing.T) {
	if parentIDSQL(nil) != nil {
		t.Fatal("nil ParentID => SQL NULL")
	}
	v := uint64(7)
	if parentIDSQL(&v) != uint64(7) {
		t.Fatalf("got %v", parentIDSQL(&v))
	}
	a, b := uint64(1), uint64(1)
	if !parentIDEqual(nil, nil) || parentIDEqual(&a, nil) || !parentIDEqual(&a, &b) {
		t.Fatal("parentIDEqual mismatch")
	}
}
