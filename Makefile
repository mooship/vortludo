.PHONY: build dev clean prod deps test setup

# Development mode
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
	go build -tags netgo -ldflags '-s -w' -o vortludo

# Run in production mode without rebuild
run:
	@echo "🚀 Starting production server..."
	GIN_MODE=release ./vortludo.exe || GIN_MODE=release ./vortludo

# Clean all build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f vortludo.exe vortludo
	@rm -f data/daily-word.json
	@rm -rf data/sessions

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
