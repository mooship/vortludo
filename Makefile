.PHONY: build dev clean prod deps test setup build-assets

# Development mode - run without minification
dev:
	@echo "🔧 Starting development server..."
	go run .

# Build and minify static assets
build-assets:
	@echo "🎨 Building and minifying static assets..."
	@mkdir -p dist/static dist/templates
	@cp -r static/* dist/static/
	@echo "📦 Minifying CSS..."
	@go run cmd/minify/main.go -type=css -input=static/style.css -output=dist/static/style.css
	@echo "📦 Minifying JavaScript..."
	@go run cmd/minify/main.go -type=js -input=static/client.js -output=dist/static/client.js
	@echo "📦 Minifying HTML templates..."
	@for template in templates/*.html; do \
		filename=$$(basename "$$template"); \
		go run cmd/minify/main.go -type=html -input="$$template" -output="dist/templates/$$filename"; \
	done
	@echo "✅ Asset minification complete!"

# Build application with minified assets
build: build-assets
	@echo "🔨 Building application..."
	go build -o vortludo.exe .

# Production mode - build and run with minified assets
prod: build
	@echo "🚀 Starting production server..."
	GIN_MODE=release ./vortludo.exe || GIN_MODE=release ./vortludo

# Build for Render deployment with minified assets
render-build: build-assets
	@echo "🚀 Building for Render deployment..."
	go build -tags netgo -ldflags '-s -w' -o vortludo

# Clean all build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f vortludo.exe vortludo
	@rm -f data/daily-word.json
	@rm -rf data/sessions dist

# Install dependencies
deps:
	@echo "📥 Installing dependencies..."
	go mod tidy && go mod download

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Setup project for first time
setup: deps
	@echo "🚀 Setting up project..."
	@mkdir -p data
	@echo "✅ Project setup complete! Run 'make dev' to start development server."
