.PHONY: build dev clean run prod deps test setup

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

# Clean build artifacts and data
clean:
	@echo "🧹 Cleaning build artifacts..."
	@if exist vortludo.exe del vortludo.exe
	@if exist data\daily-word.json del data\daily-word.json
	@if exist data\sessions rmdir /s /q data\sessions

# Run in production mode (no minification)
run:
	@echo "🔧 Running in production mode..."
	set ENV=production && go run .

# Install dependencies
deps:
	@echo "📥 Installing dependencies..."
	go mod tidy
	go mod download

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Setup project for first time
setup: deps
	@echo "🚀 Setting up project..."
	@if not exist data mkdir data
	@echo "✅ Project setup complete! Run 'make dev' to start development server."
