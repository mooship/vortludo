# Vortludo 🟩🟨⬜

A fun, open-source Wordle-inspired game built with Go! 🎮

## Features ✨

-   Guess the hidden word in 6 tries
-   Color-coded feedback for each guess
-   Web-based interface
-   Custom word lists

## Getting Started 🚀

### Prerequisites

-   Go 1.24 or newer

### Running Locally

```sh
# Clone the repository
git clone https://github.com/mooship/vortludo.git
cd vortludo

# Run the server
go run .
```

Then open your browser and go to [http://localhost:8080](http://localhost:8080) 🌐

### Live Reloading with Air (Windows & Cross-Platform)

This project includes a preconfigured [Air](https://github.com/air-verse/air) setup for fast Go development with live reloading, and is fully compatible with Windows. The configuration is in `.air.toml` and uses a Windows-friendly build command by default.

**To use Air:**

1. [Install Air](https://github.com/air-verse/air#installation):

    ```sh
    # macOS/Linux

    curl -sSfL https://raw.githubusercontent.com/air-verse/air/master/install.sh | sh

    # Windows (Powershell)

    iwr -useb https://raw.githubusercontent.com/air-verse/air/master/install.ps1 | iex

    # Install via Go (Go 1.20+)

    go install github.com/air-verse/air@latest

    # On Windows you may need to add your Go bin to PATH, e.g. add %USERPROFILE%\go\bin so the `air` command is runnable
    ```

2. Start the dev server with live reload:

    ```sh
    air
    ```

Air will watch for changes in Go and HTML files, rebuild, and restart the server automatically. The default `.air.toml` is set up for Windows, but can be easily adapted for other platforms if needed.

## Project Structure 🗂️

-   `main.go`: Main application entrypoint.
-   `handlers.go`: HTTP handlers for different routes.
-   `game.go`: Core game logic.
-   `session.go`: Manages game sessions.
-   `middleware.go`: Defines middleware for logging and other tasks.
-   `constants.go`: Holds application constants.
-   `types.go`: Defines data structures.
-   `util.go`: Contains utility functions.
-   `static/`: Holds all static assets like CSS, JavaScript, and favicons.
-   `templates/`: Contains HTML templates for the web interface.
-   `data/`: Includes word lists used in the game.
-   `.air.toml`: Configuration file for Air, a live-reloading tool.
-   `go.mod`, `go.sum`: Manage project dependencies.

## Contributing 🤝

Pull requests are welcome! For major changes, please open an issue first to discuss what you would like to change.

## License 📄

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). See the [LICENSE](LICENSE) file for details.

Enjoy playing Vortludo! 🧩
