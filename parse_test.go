package main

import (
	"reflect"
	"testing"
)

func TestExpandCPUList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{name: "single", input: "0", want: []int{0}},
		{name: "range", input: "0-3", want: []int{0, 1, 2, 3}},
		{name: "comma-separated", input: "0,2,4", want: []int{0, 2, 4}},
		{name: "mixed", input: "0-1,4,8-10", want: []int{0, 1, 4, 8, 9, 10}},
		{name: "spaces-as-commas", input: "0-1 4 8-10", want: []int{0, 1, 4, 8, 9, 10}},
		{name: "whitespace-trimmed", input: " 0 , 1 ", want: []int{0, 1}},
		{name: "large-range", input: "128-131", want: []int{128, 129, 130, 131}},
		{name: "empty", input: "", wantErr: true},
		{name: "invalid-number", input: "abc", wantErr: true},
		{name: "invalid-range-start", input: "abc-3", wantErr: true},
		{name: "invalid-range-end", input: "0-abc", wantErr: true},
		{name: "reversed-range", input: "3-0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandCPUList(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandCPUList(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("expandCPUList(%q) unexpected error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandCPUList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizePCI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "already-normalized", input: "0000:3b:00.0", want: "0000:3b:00.0"},
		{name: "eight-digit-domain", input: "00000000:3B:00.0", want: "0000:3b:00.0"},
		{name: "uppercase", input: "0000:3B:00.0", want: "0000:3b:00.0"},
		{name: "whitespace", input: " 0000:3B:00.0 ", want: "0000:3b:00.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePCI(tt.input)
			if got != tt.want {
				t.Errorf("normalizePCI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
