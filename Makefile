.PHONY: build dev clean run prod deps

# Development mode - run without minification
dev:
	@echo "🔧 Starting development server..."
	go run .

# Build application
build:
	@echo "🔨 Building application..."
	go build -o vortludo.exe .

# Production mode - build and run
prod: build
	@echo "🚀 Starting production server..."
	set GIN_MODE=release && vortludo.exe

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@if exist vortludo.exe del vortludo.exe

# Run in production mode (no minification)
run:
	@echo "🔧 Running in production mode..."
	set ENV=production && go run .

# Install dependencies
deps:
	@echo "📥 Installing dependencies..."
	go mod tidy
	go mod download
