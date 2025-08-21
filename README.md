
# Vortludo 🟩🟨⬜

A fun, open-source Wordle-inspired game built with Go! 🎮

## Features ✨

- Guess the hidden word in 6 tries
- Color-coded feedback for each guess
- Web-based interface
- Custom word lists

## Getting Started 🚀

### Prerequisites

- Go 1.24 or newer

### Running Locally

```sh
# Clone the repository
git clone https://github.com/mooship/vortludo.git
cd vortludo

# Run the server
go run ./cmd/vortludo
```

Then open your browser and go to [http://localhost:8080](http://localhost:8080) 🌐

## Project Structure 🗂️

- `cmd/vortludo/` – Main application entrypoint
- `internal/types/` – Game types and logic
- `static/` – JS, CSS, and icons
- `templates/` – HTML templates
- `data/` – Word lists

## Contributing 🤝

Pull requests are welcome! For major changes, please open an issue first to discuss what you would like to change.

## License 📄

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). See the [LICENSE](LICENSE) file for details.

Enjoy playing Vortludo! 🧩
