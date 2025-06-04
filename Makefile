.PHONY: build dev clean minify run prod

# Development mode - run without minification
dev:
	@echo "🔧 Starting development server..."
	go run .

# Build and minify assets
build: minify
	@echo "🔨 Building application..."
	go build -o vortludo.exe .

# Minify assets only
minify:
	@echo "📦 Minifying assets..."
	go run build.go

# Production mode - build and run with minified assets
prod: build
	@echo "🚀 Starting production server..."
	set GIN_MODE=release && vortludo.exe

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@if exist dist rmdir /s /q dist
	@if exist vortludo.exe del vortludo.exe

# Run with minified assets in development
run: minify
	@echo "🔧 Running with minified assets..."
	set ENV=production && go run .

# Install dependencies
deps:
	@echo "📥 Installing dependencies..."
	go mod tidy
	go mod download
