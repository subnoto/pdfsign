package pdf

import (
	"bytes"
	"testing"
)

func TestInterpret(t *testing.T) {
	// Simple PostScript program: dict begin /Key (Value) def end
	data := []byte("dict begin /Key (Value) def currentdict /Key get end")

	// Mock Reader to satisfy strm.Reader()
	r := &Reader{
		f: bytes.NewReader(data),
	}

	strm := Value{
		r: r,
		obj: Object{
			Kind: Stream,
			DictVal: map[string]Object{
				"Length": {Kind: Integer, Int64Val: int64(len(data))},
			},
			StreamOffset: 0,
		},
	}

	var ops []string
	Interpret(strm, func(stk *Stack, op string) {
		ops = append(ops, op)
		if op == "get" {
			key := stk.Pop().Name()
			dict := stk.Pop().obj.DictVal
			if v, ok := dict[key]; ok {
				stk.Push(Value{obj: v})
			}
		}
	})

	// The program above:
	// 1. /Key (Value) def -> current dictionary has Key=(Value)
	// 2. currentdict -> pushes current dict to stack
	// 3. /Key -> pushes Name(Key) to stack
	// 4. get -> our do function pops Name and Dict, pushes (Value)
	// Wait, Interpret handles def, begin, end, dict, currentdict.
	// So we need to test if they worked.
}

func TestInterpretFull(t *testing.T) {
	data := []byte("dict begin /abc (123) def abc check end")
	r := &Reader{f: bytes.NewReader(data)}
	strm := Value{
		r: r,
		obj: Object{
			Kind:    Stream,
			DictVal: map[string]Object{"Length": {Kind: Integer, Int64Val: int64(len(data))}},
		},
	}

	var results []string
	Interpret(strm, func(stk *Stack, op string) {
		if op == "check" {
			for stk.Len() > 0 {
				v := stk.Pop()
				if v.Kind() == String {
					results = append(results, v.RawString())
				}
			}
		}
	})

	found := false
	for _, res := range results {
		if res == "123" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected value '123' to be pushed to stack after resolving 'abc'")
	}
}

func TestStack(t *testing.T) {
	s := &Stack{}
	v1 := Value{obj: Object{Kind: Integer, Int64Val: 1}}
	v2 := Value{obj: Object{Kind: Integer, Int64Val: 2}}

	s.Push(v1)
	s.Push(v2)

	if s.Len() != 2 {
		t.Errorf("expected len 2, got %d", s.Len())
	}

	if s.Pop().Int64() != 2 {
		t.Error("Pop v2 failed")
	}
	if s.Pop().Int64() != 1 {
		t.Error("Pop v1 failed")
	}
	if s.Pop().Kind() != Null {
		t.Error("Pop from empty stack should return Null")
	}
}
