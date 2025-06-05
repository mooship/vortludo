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
	GIN_MODE=release ./vortludo.exe || GIN_MODE=release ./vortludo

# Build for Render deployment
render-build:
	@echo "🚀 Building for Render deployment..."
	go build -tags netgo -ldflags '-s -w' -o app

# Clean build artifacts and data
clean:
	@echo "🧹 Cleaning build artifacts..."
	@if [ -f vortludo.exe ]; then rm vortludo.exe; fi; \
	if [ -f vortludo ]; then rm vortludo; fi; \
	if [ -f app ]; then rm app; fi; \
	if [ -f data/daily-word.json ]; then rm data/daily-word.json; fi; \
	if [ -d data/sessions ]; then rm -rf data/sessions; fi

# Run in production mode (no minification)
run:
	@echo "🔧 Running in production mode..."
	ENV=production go run .

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
	@if [ ! -d data ]; then mkdir -p data; fi
	@echo "✅ Project setup complete! Run 'make dev' to start development server."
