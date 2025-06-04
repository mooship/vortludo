// Handle shake animation for invalid word errors
document.body.addEventListener('htmx:afterSwap', function(evt) {
    const errorAlert = document.querySelector('.alert-danger');
    if (errorAlert && (errorAlert.textContent.includes('Not in word list') || errorAlert.textContent.includes('Word must be 5 letters'))) {
        if (window.Alpine) {
            const alpineData = Alpine.$data(document.querySelector('[x-data]'));
            if (alpineData) {
                const rows = document.querySelectorAll('.guess-row');
                if (rows[alpineData.currentRow]) {
                    rows[alpineData.currentRow].classList.add('shake');
                    setTimeout(() => {
                        rows[alpineData.currentRow].classList.remove('shake');
                    }, 500);
                }
            }
        }
    }
});

// Preserve tile content during HTMX updates
document.body.addEventListener('htmx:beforeSwap', function(evt) {
    // Store current input state before swap
    if (window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData && alpineData.currentGuess) {
            // Store the current guess temporarily
            window.tempCurrentGuess = alpineData.currentGuess;
            window.tempCurrentRow = alpineData.currentRow;
        }
    }
});

// Restore current input state after HTMX swap (e.g. invalid guess)
document.body.addEventListener('htmx:afterSwap', function(evt) {
    if (window.tempCurrentGuess && window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData) {
            // put the guess back into Alpine and redraw
            alpineData.currentGuess = window.tempCurrentGuess;
            alpineData.currentRow   = window.tempCurrentRow;
            alpineData.updateDisplay();
        }
        // clear temps
        window.tempCurrentGuess = null;
        window.tempCurrentRow   = null;
    }
});

// Prevent zoom gestures on mobile
document.addEventListener('gesturestart', function(e) {
    e.preventDefault();
});

// Prevent double-tap zoom
let lastTouchEnd = 0;
document.addEventListener('touchend', function (event) {
    const now = (new Date()).getTime();
    if (now - lastTouchEnd <= 300) {
        event.preventDefault();
    }
    lastTouchEnd = now;
}, false);

window.gameApp = function() {
    return {
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: {},

        initGame() {
            // Initialize theme
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-theme', savedTheme);

            // Initialize game state
            this.updateGameState();
        },

        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-theme', theme);
            localStorage.setItem('theme', theme);
        },

        handleKeyPress(e) {
            if (this.gameOver) return;

            if (e.key === 'Enter') {
                this.submitGuess();
            } else if (e.key === 'Backspace') {
                this.deleteLetter();
            } else if (/^[a-zA-Z]$/.test(e.key)) {
                this.addLetter(e.key.toUpperCase());
            }
        },

        handleVirtualKey(key) {
            if (this.gameOver) return;

            if (key === 'ENTER') {
                this.submitGuess();
            } else if (key === 'BACKSPACE') {
                this.deleteLetter();
            } else {
                this.addLetter(key);
            }
        },

        addLetter(letter) {
            if (this.currentGuess.length < 5) {
                this.currentGuess += letter;
                this.updateDisplay();
            }
        },

        deleteLetter() {
            if (this.currentGuess.length > 0) {
                this.currentGuess = this.currentGuess.slice(0, -1);
                this.updateDisplay();
            }
        },

        updateDisplay() {
            const rows = document.querySelectorAll('.guess-row');
            const row = rows[this.currentRow];
            if (!row) return;
            const tiles = row.querySelectorAll('.tile');
            tiles.forEach((tile, i) => {
                const letter = this.currentGuess[i] || '';
                tile.textContent = letter;
                if (letter) {
                    tile.classList.add('filled');
                } else {
                    tile.classList.remove('filled');
                }
            });
        },

        updateGameState() {
            const board = document.getElementById('game-board');
            if (!board) return;

            // Clear current guess after any submission (valid or invalid)
            this.currentGuess = '';

            // Check if game is over
            this.gameOver = board.querySelector('.game-over-container') !== null;

            // Find the current row by looking for the first row without any filled tiles
            const rows = board.querySelectorAll('.guess-row');
            let foundCurrentRow = false;

            rows.forEach((row, index) => {
                const tiles = row.querySelectorAll('.tile');
                const hasContent = Array.from(tiles).some(tile => tile.textContent.trim() !== '');
                const activeTiles = row.querySelectorAll('.tile.active');

                // Find the first empty row or the row with active tiles
                if (!hasContent && !foundCurrentRow && !this.gameOver) {
                    this.currentRow = index;
                    foundCurrentRow = true;
                } else if (activeTiles.length > 0 && !foundCurrentRow) {
                    this.currentRow = index;
                    foundCurrentRow = true;
                }
            });

            if (!foundCurrentRow && !this.gameOver) {
                // If all rows have content, we might be at the end
                this.currentRow = Math.min(5, rows.length);
            }

            this.updateKeyboardColors();
            this.animateNewGuess();
            this.checkForWin();
        },

        submitGuess() {
            if (this.currentGuess.length === 5) {
                const rows = document.querySelectorAll('.guess-row');
                if (rows[this.currentRow]) {
                    rows[this.currentRow].classList.add('submitting');
                }

                document.getElementById('guess-input').value = this.currentGuess;
                document.getElementById('guess-form').dispatchEvent(new Event('submit'));
            }
        },

        updateKeyboardColors() {
            const tiles = document.querySelectorAll('.tile.filled');
            this.keyStatus = {};

            tiles.forEach(tile => {
                // Skip invalid tiles for keyboard coloring
                if (tile.classList.contains('invalid')) return;
                
                const letter = tile.textContent;
                const status = tile.classList.contains('correct') ? 'correct' :
                               tile.classList.contains('present') ? 'present' :
                               tile.classList.contains('absent') ? 'absent' : '';

                if (letter && status) {
                    if (!this.keyStatus[letter] ||
                        status === 'correct' ||
                        (status === 'present' && this.keyStatus[letter] === 'absent')) {
                        this.keyStatus[letter] = status;
                    }
                }
            });
        },

        getKeyClass(letter) {
            return this.keyStatus[letter] || '';
        },

        animateNewGuess() {
            const rows = document.querySelectorAll('.guess-row');
            const lastFilledRow = Array.from(rows).find((row, index) => {
                const filledTiles = row.querySelectorAll('.tile.filled');
                const invalidTiles = row.querySelectorAll('.tile.invalid');
                // Don't animate invalid guesses
                return filledTiles.length === 5 &&
                       invalidTiles.length === 0 &&
                       !row.classList.contains('animated') &&
                       (row.classList.contains('submitting') || index < this.currentRow);
            });

            if (lastFilledRow) {
                const tiles = lastFilledRow.querySelectorAll('.tile.filled');
                tiles.forEach((tile, index) => {
                    const letter = tile.textContent;
                    tile.style.setProperty('--tile-index', index);

                    setTimeout(() => {
                        if (tile.textContent !== letter) {
                            tile.textContent = letter;
                        }
                        tile.classList.add('flip');
                    }, index * 100);
                });
                lastFilledRow.classList.add('animated');
                lastFilledRow.classList.remove('submitting');
            }
        },

        checkForWin() {
            const rows = document.querySelectorAll('.guess-row');
            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile.correct');
                if (tiles.length === 5 && !row.classList.contains('winner')) {
                    row.classList.add('winner');
                    tiles.forEach((tile, index) => {
                        tile.style.setProperty('--tile-index', index);
                    });
                }
            });
        },

        shareResults() {
            const rows = document.querySelectorAll('.guess-row');
            let emojiGrid = 'VORTLUDO\n\n';

            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile.filled');
                if (tiles.length === 5) {
                    tiles.forEach(tile => {
                        if (tile.classList.contains('correct')) {
                            emojiGrid += '🟩';
                        } else if (tile.classList.contains('present')) {
                            emojiGrid += '🟨';
                        } else {
                            emojiGrid += '⬜';
                        }
                    });
                    emojiGrid += '\n';
                }
            });

            this.copyToClipboard(emojiGrid.trim());
        },

        async copyToClipboard(text) {
            try {
                await navigator.clipboard.writeText(text);
                alert("Results copied to clipboard!");
            } catch (err) {
                const textarea = document.createElement('textarea');
                textarea.value = text;
                textarea.style.position = 'fixed';
                document.body.appendChild(textarea);
                textarea.select();
                try {
                    document.execCommand('copy');
                    alert("Results copied to clipboard!");
                } catch {
                    alert("Failed to copy to clipboard.");
                }
                document.body.removeChild(textarea);
            }
        }
    };
}

// Expose shareResults globally for the template button
window.shareResults = function() {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};