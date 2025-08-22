package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadWordListFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var words []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		if w != "" {
			words = append(words, strings.ToUpper(w))
		}
	}
	return words, scanner.Err()
}

func TestAcceptedWordsNoDuplicates(t *testing.T) {
	words, err := loadWordListFromFile("data/accepted_words.txt")
	if err != nil {
		t.Fatalf("failed to load accepted_words.txt: %v", err)
	}
	seen := make(map[string]struct{})
	for _, w := range words {
		if _, ok := seen[w]; ok {
			t.Errorf("duplicate word in accepted_words.txt: %s", w)
		}
		seen[w] = struct{}{}
	}
}

func TestWordsNoDuplicates(t *testing.T) {
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	seen := make(map[string]struct{})
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		if _, ok := seen[w]; ok {
			t.Errorf("duplicate word in words.json: %s", w)
		}
		seen[w] = struct{}{}
	}
}

func TestAllWordsInAcceptedList(t *testing.T) {
	accepted, err := loadWordListFromFile("data/accepted_words.txt")
	if err != nil {
		t.Fatalf("failed to load accepted_words.txt: %v", err)
	}
	acceptedSet := make(map[string]struct{}, len(accepted))
	for _, w := range accepted {
		acceptedSet[w] = struct{}{}
	}
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		if _, ok := acceptedSet[w]; !ok {
			t.Errorf("word in words.json not found in accepted_words.txt: %s", w)
		}
	}
}

func TestAllWordsHaveHints(t *testing.T) {
	f, err := os.Open("data/words.json")
	if err != nil {
		t.Fatalf("failed to open words.json: %v", err)
	}
	defer f.Close()
	var wordList struct {
		Words []struct {
			Word string `json:"word"`
			Hint string `json:"hint"`
		}
	}
	if err := json.NewDecoder(f).Decode(&wordList); err != nil {
		t.Fatalf("failed to decode words.json: %v", err)
	}
	for _, entry := range wordList.Words {
		w := strings.ToUpper(strings.TrimSpace(entry.Word))
		hint := strings.TrimSpace(entry.Hint)
		if hint == "" {
			t.Errorf("word in words.json missing hint: %s", w)
		}
	}
}
