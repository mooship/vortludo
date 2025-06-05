package main

import (
	"testing"
)

func TestCheckGuess(t *testing.T) {
	target := "APPLE"
	tests := []struct {
		guess   string
		want    []GuessResult
		comment string
	}{
		{
			guess: "APPLE",
			want: []GuessResult{
				{"A", "correct"},
				{"P", "correct"},
				{"P", "correct"},
				{"L", "correct"},
				{"E", "correct"},
			},
			comment: "All correct",
		},
		{
			guess: "ALLEY",
			want: []GuessResult{
				{"A", "correct"},
				{"L", "present"},
				{"L", "absent"},
				{"E", "present"},
				{"Y", "absent"},
			},
			comment: "Mix of correct, present, absent",
		},
		{
			guess: "ZZZZZ",
			want: []GuessResult{
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
				{"Z", "absent"},
			},
			comment: "All absent",
		},
	}

	for _, tt := range tests {
		got := checkGuess(tt.guess, target)
		for i := range got {
			if got[i].Letter != tt.want[i].Letter || got[i].Status != tt.want[i].Status {
				t.Errorf("%s: guess %s, pos %d: got %+v, want %+v", tt.comment, tt.guess, i, got[i], tt.want[i])
			}
		}
	}
}

func TestIsValidWord(t *testing.T) {
	wordSet = map[string]struct{}{
		"APPLE": {},
		"BANJO": {},
	}
	tests := []struct {
		word string
		want bool
	}{
		{"APPLE", true},
		{"BANJO", true},
		{"PEACH", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isValidWord(tt.word)
		if got != tt.want {
			t.Errorf("isValidWord(%q) = %v, want %v", tt.word, got, tt.want)
		}
	}
}

func TestNormalizeGuess(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"apple", "APPLE"},
		{"  banjo ", "BANJO"},
		{"PeAch", "PEACH"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeGuess(tt.input)
		if got != tt.want {
			t.Errorf("normalizeGuess(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
