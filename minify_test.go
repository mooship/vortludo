package main

import (
	"strings"
	"testing"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

// TestHTMLMinification checks that HTML is minified as expected
func TestHTMLMinification(t *testing.T) {
	m := minify.New()
	m.AddFunc("text/html", html.Minify)

	input := `<html>
	<head>
		<title>Test</title>
	</head>
	<body>
		<p> Hello   World! </p>
	</body>
</html>`
	expected := `<title>Test</title><p>Hello World!`

	var b strings.Builder
	err := m.Minify("text/html", &b, strings.NewReader(input))
	if err != nil {
		t.Fatalf("HTML minification failed: %v", err)
	}
	got := b.String()
	// Remove whitespace for comparison
	got = strings.ReplaceAll(got, "\n", "")
	expected = strings.ReplaceAll(expected, "\n", "")
	if got != expected {
		t.Errorf("HTML minification mismatch:\nGot:      %q\nExpected: %q", got, expected)
	}
}

// TestCSSMinification checks that CSS is minified as expected
func TestCSSMinification(t *testing.T) {
	m := minify.New()
	m.AddFunc("text/css", css.Minify)

	input := `
		body {
			color: #fff;
			margin: 0  ;
		}
	`
	expected := `body{color:#fff;margin:0}`

	var b strings.Builder
	err := m.Minify("text/css", &b, strings.NewReader(input))
	if err != nil {
		t.Fatalf("CSS minification failed: %v", err)
	}
	got := b.String()
	if got != expected {
		t.Errorf("CSS minification mismatch:\nGot:      %q\nExpected: %q", got, expected)
	}
}

// TestJSMinification checks that JavaScript is minified as expected
func TestJSMinification(t *testing.T) {
	m := minify.New()
	m.AddFunc("application/javascript", js.Minify)

	input := `
		function add(a, b) {
			return a + b;
		}
	`
	expected := `function add(e,t){return e+t}` // The minifier outputs this format

	var b strings.Builder
	err := m.Minify("application/javascript", &b, strings.NewReader(input))
	if err != nil {
		t.Fatalf("JS minification failed: %v", err)
	}
	got := b.String()
	if got != expected {
		t.Errorf("JS minification mismatch:\nGot:      %q\nExpected: %q", got, expected)
	}
}
