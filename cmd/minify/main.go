package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

func main() {
	var (
		inputFile  = flag.String("input", "", "Input file path")
		outputFile = flag.String("output", "", "Output file path")
		fileType   = flag.String("type", "", "File type (CSS, JS, or HTML)")
	)
	flag.Parse()

	if *inputFile == "" || *outputFile == "" || *fileType == "" {
		log.Fatal("Usage: go run cmd/minify/main.go -input=<file> -output=<file> -type=<css|js|html>")
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(*outputFile), 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Read input file
	input, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	// Setup minifier
	m := minify.New()

	switch strings.ToLower(*fileType) {
	case "css":
		m.AddFunc("text/css", css.Minify)
		minified, err := m.String("text/css", string(input))
		if err != nil {
			log.Fatalf("Failed to minify CSS: %v", err)
		}

		// Write minified output
		if err := os.WriteFile(*outputFile, []byte(minified), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}

	case "js":
		m.AddFunc("application/javascript", js.Minify)
		minified, err := m.String("application/javascript", string(input))
		if err != nil {
			log.Fatalf("Failed to minify JavaScript: %v", err)
		}

		// Write minified output
		if err := os.WriteFile(*outputFile, []byte(minified), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}

	case "html":
		m.AddFunc("text/html", html.Minify)
		minified, err := m.String("text/html", string(input))
		if err != nil {
			log.Fatalf("Failed to minify HTML: %v", err)
		}

		// Write minified output
		if err := os.WriteFile(*outputFile, []byte(minified), 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}

	default:
		log.Fatalf("Unsupported file type: %s (supported: css, js, html)", *fileType)
	}

	fmt.Printf("Successfully minified %s -> %s\n", *inputFile, *outputFile)
}
