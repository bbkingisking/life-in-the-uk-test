package ordered

import (
	"encoding/json"
	"testing"
)

func TestMapPreservesInsertionOrder(t *testing.T) {
	m := NewMap[int]()
	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("c", 3)

	got, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"b":2,"a":1,"c":3}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestMapSetUpdatesInPlace(t *testing.T) {
	m := NewMap[int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 99) // update, not a new key

	if got := m.Keys(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("keys = %v, want [a b]", got)
	}
	got, _ := json.Marshal(m)
	if want := `{"a":99,"b":2}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFromMapNumericOrdersByIntegerValue(t *testing.T) {
	m := map[string]string{"10": "ten", "2": "two", "1": "one"}
	om := FromMapNumeric(m)

	got, err := json.Marshal(om)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"1":"one","2":"two","10":"ten"}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestMarshalIndentPreservesOrder(t *testing.T) {
	m := NewMap[int]()
	m.Set("10", 1)
	m.Set("2", 2)

	got, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"10\": 1,\n  \"2\": 2\n}"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
