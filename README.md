# Vortludo 🎯

A libre (free and open source) Wordle clone built with Go and Gin. Each game session uses a random word from the dictionary, making it replayable with fresh challenges!

## Features

- 🎮 **Classic Wordle gameplay** - Guess the 5-letter word in 6 tries
- 🔀 **Random words** - Each new game picks a different word from the dictionary
- 💡 **Helpful hints** - Each word comes with a hint to guide you
- 📱 **Responsive design** - Works on desktop and mobile
- 💾 **Session persistence** - Games are saved across browser sessions
- 🌙 **Automatic cleanup** - Old game sessions are cleaned up automatically
- 🚀 **Zero database** - Simple file-based storage
- 🔒 **Session security** - HTTPOnly cookies and session validation

## Quick Start

### Prerequisites

- Go 1.24 or higher
- Make (optional, for convenience scripts)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/mooship/vortludo.git
   cd vortludo
   ```

2. **Setup the project**
   ```bash
   make setup
   ```

3. **Start development server**
   ```bash
   make dev
   ```

4. **Open your browser**
   Navigate to `http://localhost:8080`

## Development Scripts

We use Make for convenient development workflows:

```bash
# Development
make dev          # Start development server
make deps         # Install/update dependencies
make setup        # First-time project setup

# Building
make build        # Build binary for current OS
make render-build # Build for Render deployment
make prod         # Build and run in production mode
make run          # Run in production mode without rebuild

# Maintenance
make test         # Run tests
make clean        # Clean build artifacts and game data
```

### Manual Commands (without Make)

```bash
# Development
go run .

# Install dependencies
go mod tidy && go mod download

# Build for local use
go build -o vortludo.exe .

# Build for Render deployment
go build -tags netgo -ldflags '-s -w' -o vortludo

# Run tests
go test -v ./...
```

## Project Structure

```
vortludo/
├── main.go              # Main application and HTTP handlers
├── types.go             # Data structures and types
├── persistence.go       # File-based game session storage
├── data/
│   ├── words.json       # Dictionary of valid words with hints
│   ├── daily-word.json  # Current daily word (auto-generated)
│   └── sessions/        # Game session files (auto-generated)
├── templates/           # HTML templates
├── static/             # CSS, JS, and favicon assets
├── render.yaml         # Render.com deployment configuration
├── Makefile           # Development and build scripts
└── .github/workflows/ # GitHub Actions CI/CD
```

## How It Works

### Game Logic

1. **Word Selection**: Each new game randomly selects a word from `data/words.json`
2. **Session Management**: Games are tied to browser sessions via HTTPOnly cookies
3. **Persistence**: Game state is saved to JSON files in `data/sessions/`
4. **Cleanup**: Old sessions (>2 hours) are automatically removed every hour

### File-Based Storage

Instead of a database, Vortludo uses simple JSON files:

- **Game sessions**: `data/sessions/{sessionId}.json`
- **Word dictionary**: `data/words.json` (static)
- **Daily word**: `data/daily-word.json` (rotates daily at midnight)

This approach is:
- ✅ Simple and lightweight
- ✅ Easy to backup and restore
- ✅ No database setup required
- ✅ Perfect for single-server deployments
- ✅ Automatic cleanup prevents disk bloat

### Session Lifecycle

1. **Creation**: New session gets a unique ID and random word
2. **Gameplay**: Guesses are validated against dictionary and stored with color feedback
3. **Persistence**: State is saved to both memory and file after each guess
4. **Cleanup**: Sessions older than 2 hours are automatically deleted

## Configuration

### Environment Variables

- `PORT` - Server port (default: 8080)
- `GIN_MODE` - Set to "release" for production optimizations
- `ENV` - Set to "production" for production static file serving

### Cache Control

- **Development**: All caching disabled for live reloading
- **Production**: Static assets cached for 24 hours, HTML/API not cached

## Deployment

### Render.com Deployment

1. **Connect your GitHub repository** to Render
2. **Create a new Web Service** with these settings:
   - **Runtime**: `Go`
   - **Build Command**: `go build -tags netgo -ldflags '-s -w' -o vortludo`
   - **Start Command**: `./vortludo`
3. **Set Environment Variables**:
   - `GIN_MODE=release`
   - `ENV=production`
   - `PORT=10000`
4. **Deploy** - Render will automatically build and deploy your app

Alternatively, you can use the included `render.yaml` file for automatic configuration by connecting your repo and Render will detect it automatically.

Your app will be available at `https://your-app-name.onrender.com`

### Local Production Testing

```bash
# Build for production (same as Render)
make render-build

# Test production build locally
GIN_MODE=release PORT=8080 ENV=production ./vortludo
```

### Other Platforms

The application can be deployed to any platform that supports Go:
- Heroku
- Railway
- Fly.io
- DigitalOcean App Platform
- Traditional VPS with systemd

## API Endpoints

- `GET /` - Main game page
- `GET /new-game` - Start a new game (redirects)
- `POST /new-game` - Start a new game (form submission)
- `POST /guess` - Submit a word guess (returns HTMX partial)
- `GET /game-state` - Get current game state (HTMX partial)
- `GET /static/*` - Static assets (CSS, JS, images)

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go best practices and formatting (`go fmt`)
- Add tests for new functionality
- Update documentation as needed
- Test on both development and production modes
- Ensure proper error handling and logging

## Technology Stack

- **Backend**: Go 1.21+ with Gin web framework
- **Frontend**: HTML5, CSS3, vanilla JavaScript
- **UI Framework**: [Bootstrap 5](https://getbootstrap.com/) (CDN)
- **Reactive UI**: [Alpine.js](https://alpinejs.dev/) (CDN)
- **AJAX/Partial Updates**: [HTMX](https://htmx.org/) (CDN)
- **Storage**: JSON files (no database required)
- **Templating**: Go's html/template
- **Build Tools**: Make + Go modules
- **Deployment**: Render.com with GitHub Actions

## License

This project is open source and available under the MIT License.

## Acknowledgments

- Inspired by the original Wordle game by Josh Wardle
- Built as a libre (free and open source) alternative
- Word list curated for family-friendly gameplay
- Community contributions welcome

---

**Have fun playing! 🎯**
