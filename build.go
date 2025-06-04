//go:build ignore

package main

import (
	"fmt"
	"io/fs"
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
	// Create minifier
	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("application/javascript", js.Minify)

	// Create dist directories
	if err := os.MkdirAll("dist/templates", 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll("dist/static", 0755); err != nil {
		log.Fatal(err)
	}

	// Minify HTML templates
	err := filepath.WalkDir("templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".html") {
			return minifyFile(m, path, "dist/"+path, "text/html")
		}
		return nil
	})
	if err != nil {
		log.Fatal("Error minifying templates:", err)
	}

	// Minify CSS files
	err = filepath.WalkDir("static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".css") {
			return minifyFile(m, path, "dist/"+path, "text/css")
		}
		return nil
	})
	if err != nil {
		log.Fatal("Error minifying CSS:", err)
	}

	// Minify JS files
	err = filepath.WalkDir("static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".js") {
			return minifyFile(m, path, "dist/"+path, "application/javascript")
		}
		return nil
	})
	if err != nil {
		log.Fatal("Error minifying JS:", err)
	}

	fmt.Println("✅ Minification complete!")
	fmt.Println("📁 Minified files are in the 'dist' directory")
}

func minifyFile(m *minify.M, srcPath, dstPath, mediaType string) error {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	minified, err := m.Bytes(mediaType, src)
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	err = os.WriteFile(dstPath, minified, 0644)
	if err != nil {
		return err
	}

	// Calculate compression ratio
	originalSize := len(src)
	minifiedSize := len(minified)
	ratio := float64(originalSize-minifiedSize) / float64(originalSize) * 100

	fmt.Printf("📦 %s: %d bytes → %d bytes (%.1f%% reduction)\n",
		srcPath, originalSize, minifiedSize, ratio)

	return nil
}
