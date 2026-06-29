package pdf

import (
	"bytes"
	"fmt"
	"testing"
)

func TestObjectKind(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want Kind
	}{
		{"Null", Value{obj: Object{Kind: Null}}, Null},
		{"Bool", Value{obj: Object{Kind: Bool, BoolVal: true}}, Bool},
		{"Integer", Value{obj: Object{Kind: Integer, Int64Val: 42}}, Integer},
		{"Real", Value{obj: Object{Kind: Real, Float64Val: 3.14}}, Real},
		{"String", Value{obj: Object{Kind: String, StringVal: "hello"}}, String},
		{"Name", Value{obj: Object{Kind: Name, NameVal: "Type"}}, Name},
		{"Dict", Value{obj: Object{Kind: Dict, DictVal: make(map[string]Object)}}, Dict},
		{"Array", Value{obj: Object{Kind: Array, ArrayVal: []Object{}}}, Array},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.Kind(); got != tt.want {
				t.Errorf("Value.Kind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueAccessors(t *testing.T) {
	t.Run("Bool", func(t *testing.T) {
		v := Value{obj: Object{Kind: Bool, BoolVal: true}}
		if !v.Bool() {
			t.Error("Value.Bool() failed")
		}
		vErr := Value{obj: Object{Kind: Integer}}
		if vErr.Bool() {
			t.Error("Value.Bool() should return false on non-bool")
		}
	})

	t.Run("Int64", func(t *testing.T) {
		v := Value{obj: Object{Kind: Integer, Int64Val: 123}}
		if v.Int64() != 123 {
			t.Errorf("Value.Int64() = %d, want 123", v.Int64())
		}
		// Test Real to Int64 conversion
		vReal := Value{obj: Object{Kind: Real, Float64Val: 456.7}}
		if vReal.Int64() != 456 {
			t.Errorf("Value.Int64() from Real = %d, want 456", vReal.Int64())
		}
	})

	t.Run("Float64", func(t *testing.T) {
		v := Value{obj: Object{Kind: Real, Float64Val: 1.23}}
		if v.Float64() != 1.23 {
			t.Errorf("Value.Float64() = %f, want 1.23", v.Float64())
		}
		vInt := Value{obj: Object{Kind: Integer, Int64Val: 789}}
		if vInt.Float64() != 789.0 {
			t.Errorf("Value.Float64() from Integer = %f, want 789.0", vInt.Float64())
		}
	})

	t.Run("Name", func(t *testing.T) {
		v := Value{obj: Object{Kind: Name, NameVal: "Test"}}
		if v.Name() != "Test" {
			t.Errorf("Value.Name() = %q, want \"Test\"", v.Name())
		}
	})

	t.Run("String", func(t *testing.T) {
		v := Value{obj: Object{Kind: String, StringVal: "Data"}}
		if v.RawString() != "Data" {
			t.Errorf("Value.RawString() = %q, want \"Data\"", v.RawString())
		}
	})
}

func TestDictionary(t *testing.T) {
	d := make(map[string]Object)
	d["K1"] = Object{Kind: Integer, Int64Val: 1}
	d["K2"] = Object{Kind: Name, NameVal: "V2"}

	v := Value{obj: Object{Kind: Dict, DictVal: d}}

	if len(v.Keys()) != 2 {
		t.Errorf("Value.Keys() length = %d, want 2", len(v.Keys()))
	}

	if v.Key("K1").Int64() != 1 {
		t.Error("v.Key(K1) mismatch")
	}

	if v.Key("K2").Name() != "V2" {
		t.Error("v.Key(K2) mismatch")
	}

	if v.Key("NonExistent").Kind() != Null {
		t.Error("v.Key(NonExistent) should be Null")
	}
}

func TestArray(t *testing.T) {
	arr := []Object{
		{Kind: Integer, Int64Val: 10},
		{Kind: Integer, Int64Val: 20},
	}
	v := Value{obj: Object{Kind: Array, ArrayVal: arr}}

	if v.Len() != 2 {
		t.Errorf("Value.Len() = %d, want 2", v.Len())
	}

	if v.Index(0).Int64() != 10 {
		t.Error("v.Index(0) mismatch")
	}

	if v.Index(1).Int64() != 20 {
		t.Error("v.Index(1) mismatch")
	}

	if v.Index(2).Kind() != Null {
		t.Error("v.Index(2) should be Null")
	}
}

func TestValuePtrAccessors(t *testing.T) {
	ptr := objptr{id: 5, gen: 2}
	v := Value{ptr: ptr}

	if v.GetPtr().GetID() != 5 {
		t.Errorf("v.GetPtr().GetID() = %d, want 5", v.GetPtr().GetID())
	}
	if v.GetPtr().GetGen() != 2 {
		t.Errorf("v.GetPtr().GetGen() = %d, want 2", v.GetPtr().GetGen())
	}

	p := v.GetPtr()
	if p.id != 5 || p.gen != 2 {
		t.Errorf("GetPtr() = %v, want {5, 2}", p)
	}
}

func TestValueText(t *testing.T) {
	// PDFDocEncoding: \x18 is \u02d8
	v := Value{obj: Object{Kind: String, StringVal: "\x18"}}
	if v.Text() != "\u02d8" {
		t.Errorf("Text() = %q, want %q", v.Text(), "\u02d8")
	}

	// UTF-16BE: \xfe\xff\x00A is A
	vUTF16 := Value{obj: Object{Kind: String, StringVal: "\xfe\xff\x00A"}}
	if vUTF16.Text() != "A" {
		t.Errorf("Text() = %q, want %q", vUTF16.Text(), "A")
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		val  Value
		want string
	}{
		{Value{obj: Object{Kind: Null}}, "null"},
		{Value{obj: Object{Kind: Bool, BoolVal: true}}, "true"},
		{Value{obj: Object{Kind: Integer, Int64Val: 42}}, "42"},
		{Value{obj: Object{Kind: Real, Float64Val: 3.14}}, "3.14"},
		{Value{obj: Object{Kind: Name, NameVal: "Type"}}, "/Type"},
		{Value{obj: Object{Kind: String, StringVal: "hello"}}, "(hello)"},
		{Value{obj: Object{Kind: Indirect, PtrVal: objptr{id: 1, gen: 0}}}, "1 0 R"},
		{Value{obj: Object{Kind: Dict, DictVal: map[string]Object{"A": {Kind: Integer, Int64Val: 1}}}}, "<</A 1>>"},
		{Value{obj: Object{Kind: Array, ArrayVal: []Object{{Kind: Integer, Int64Val: 1}}}}, "[1]"},
		{Value{obj: Object{Kind: Stream, StreamOffset: 123}}, "<<>>@123"},
	}

	for _, tt := range tests {
		if got := tt.val.String(); got != tt.want {
			t.Errorf("Value.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestValueData(t *testing.T) {
	data := []byte("stream-data")
	r := &Reader{f: bytes.NewReader(data)}
	v := Value{
		r: r,
		obj: Object{
			Kind: Stream,
			DictVal: map[string]Object{
				"Length": {Kind: Integer, Int64Val: int64(len(data))},
			},
			StreamOffset: 0,
		},
	}

	got := v.Data()
	if string(got) != "stream-data" {
		t.Errorf("Data() = %q, want %q", string(got), "stream-data")
	}
}

func TestValueHeader(t *testing.T) {
	v := Value{
		obj: Object{
			Kind:    Stream,
			DictVal: map[string]Object{"Type": {Kind: Name, NameVal: "XRef"}},
		},
	}
	hdr := v.Header()
	if hdr.Kind() != Dict {
		t.Errorf("Header().Kind() = %v, want Dict", hdr.Kind())
	}
	if hdr.Key("Type").Name() != "XRef" {
		t.Errorf("Header().Key(Type) = %q, want XRef", hdr.Key("Type").Name())
	}
}

func TestValueReader_Error(t *testing.T) {
	errTest := fmt.Errorf("test error")
	v := Value{err: errTest}
	rd := v.Reader()
	buf := make([]byte, 10)
	n, err := rd.Read(buf)
	if n != 0 || err != errTest {
		t.Errorf("expected 0 bytes and test error, got %d bytes and %v", n, err)
	}
	if err := rd.Close(); err != errTest {
		t.Errorf("expected test error on Close, got %v", err)
	}
}

func TestValueErr(t *testing.T) {
	err := fmt.Errorf("some error")
	v := Value{err: err}
	if v.Err() != err {
		t.Errorf("v.Err() = %v, want %v", v.Err(), err)
	}
}
