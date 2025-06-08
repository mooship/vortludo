# Vortludo 🎯

[![Go](https://github.com/mooship/vortludo/actions/workflows/go.yml/badge.svg)](https://github.com/mooship/vortludo/actions/workflows/go.yml)

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

### Installation

1. **Clone the repository**

    ```bash
    git clone https://github.com/mooship/vortludo.git
    cd vortludo
    ```

2. **Install dependencies**

    ```bash
    go mod tidy && go mod download
    ```

3. **Start development server**

    ```bash
    go run .
    ```

4. **Open your browser**
   Navigate to `http://localhost:8080`

## Development with Live Reload

This project supports live reloads using [Air](https://github.com/cosmtrek/air).

### Setup

1. Install Air (one-time):

    ```
    go install github.com/cosmtrek/air@latest
    ```

2. Start the dev server with live reload:

    ```
    air
    ```

Air will watch your Go and template files and restart the server on changes.

## Building and Running

```bash
# Build for your OS
go build -o vortludo .

# Run in development mode
./vortludo

# Run in production mode
GIN_MODE=release ./vortludo
```

## Testing

Unit tests for the core game logic are provided in `core_test.go`.  
To run all tests:

```bash
go test -v ./...
```

Tests are automatically discovered and run by the GitHub Actions workflow in `.github/workflows/go.yml`.

## Project Structure

```
vortludo/
├── main.go                # Main application, HTTP handlers, and routing
├── types.go               # Data structures and types for game/session
├── persistence.go         # File-based session storage and cleanup logic
├── core_test.go           # Unit tests for core game logic and helpers
├── main_http_test.go      # HTTP handler and middleware tests
├── persistence_test.go    # Security and persistence tests (path traversal, etc)
├── minify_test.go         # Tests for minification logic
├── data/
│   ├── words.json             # Dictionary of valid words with hints
│   ├── accepted_words.json    # List of accepted guess words
│   └── sessions/              # Game session files (auto-generated)
├── static/                # CSS, JS, favicon assets
│   ├── style.css
│   ├── client.js
│   └── favicons/              # Favicon images (ico, png, apple-touch, etc)
├── templates/
│   ├── index.html             # Main HTML template
│   └── game-board.html        # Game board partial template
├── .github/
│   └── workflows/             # GitHub Actions CI/CD workflows
│       ├── go.yml
│       ├── codeql.yml
│       └── gosec.yml
├── .gitignore
├── .gitattributes
├── .prettierrc.json
├── go.mod
├── go.sum
├── LICENSE
├── SECURITY.md
├── render.yaml               # Render.com deployment configuration
└── README.md
```

- **main.go**: Entry point, HTTP server, routing, and game logic orchestration
- **types.go**: Game state, word entry, and guess result types
- **persistence.go**: Secure session file storage, loading, and cleanup
- **core_test.go**, **main_http_test.go**, **persistence_test.go**, **minify_test.go**: Comprehensive tests for logic, HTTP, security, and minification

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
go build -tags netgo -ldflags '-s -w' -o vortludo

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

- **Backend**: Go 1.24+ with Gin web framework
- **Frontend**: HTML5, CSS3, vanilla JavaScript
- **UI Framework**: [Bootstrap 5](https://getbootstrap.com/) (CDN)
- **Reactive UI**: [Alpine.js](https://alpinejs.dev/) (CDN)
- **AJAX/Partial Updates**: [HTMX](https://htmx.org/) (CDN)
- **Storage**: JSON files (no database required)
- **Templating**: Go's html/template
- **Build Tools**: Go modules
- **Deployment**: Render.com with GitHub Actions

## License

This project is open source and available under the GNU Affero General Public License v3.0 (AGPL-3.0).

See [LICENSE](./LICENSE) for details.

## Acknowledgments

- Inspired by the original Wordle game by Josh Wardle
- Built as a libre (free and open source) alternative
- Word list curated for family-friendly gameplay
- Community contributions welcome

---

**Have fun playing! 🎯**
