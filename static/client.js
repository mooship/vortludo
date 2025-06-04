/**
 * HTMX Event Handlers for Game Feedback
 */

// Shake animation for invalid word errors
document.body.addEventListener('htmx:afterSwap', function(evt) {
    const errorAlert = document.querySelector('.alert-danger');
    if (errorAlert && (errorAlert.textContent.includes('Not in word list') || errorAlert.textContent.includes('Word must be 5 letters'))) {
        if (window.Alpine) {
            const alpineData = Alpine.$data(document.querySelector('[x-data]'));
            if (alpineData) {
                const rows = document.querySelectorAll('.guess-row');
                // Target the previous row since server already incremented
                const targetRow = Math.max(0, alpineData.currentRow - 1);
                if (rows[targetRow]) {
                    rows[targetRow].classList.add('shake');
                    setTimeout(() => {
                        rows[targetRow].classList.remove('shake');
                    }, 500);
                }
            }
        }
    }
});

// Preserve current input state before HTMX swap
document.body.addEventListener('htmx:beforeSwap', function(evt) {
    if (window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData && alpineData.currentGuess) {
            window.tempCurrentGuess = alpineData.currentGuess;
            window.tempCurrentRow = alpineData.currentRow;
        }
    }
});

// Restore input state after HTMX swap for invalid guesses
document.body.addEventListener('htmx:afterSwap', function(evt) {
    if (window.tempCurrentGuess && window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData) {
            alpineData.currentGuess = window.tempCurrentGuess;
            alpineData.currentRow = window.tempCurrentRow;
            alpineData.updateDisplay();
        }
        // Clear temporary storage
        window.tempCurrentGuess = null;
        window.tempCurrentRow = null;
    }
});

/**
 * Mobile Touch Event Handlers
 */

// Prevent zoom gestures on mobile devices
document.addEventListener('gesturestart', function(e) {
    e.preventDefault();
});

// Prevent double-tap zoom on mobile devices
let lastTouchEnd = 0;
document.addEventListener('touchend', function (event) {
    const now = (new Date()).getTime();
    if (now - lastTouchEnd <= 300) {
        event.preventDefault();
    }
    lastTouchEnd = now;
}, false);

/**
 * Main Game Application using Alpine.js
 */
window.gameApp = function() {
    return {
        // Game State
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: {}, // Track keyboard color states

        /**
         * Initialize the game on page load
         */
        initGame() {
            // Load saved theme preference
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-theme', savedTheme);

            // Initialize game state from server
            this.updateGameState();
        },

        /**
         * Toggle between light and dark themes
         */
        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-theme', theme);
            localStorage.setItem('theme', theme);
        },

        /**
         * Handle physical keyboard input
         */
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

        /**
         * Handle virtual keyboard clicks
         */
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

        /**
         * Add a letter to the current guess
         */
        addLetter(letter) {
            if (this.currentGuess.length < 5) {
                this.currentGuess += letter;
                this.updateDisplay();
            }
        },

        /**
         * Remove the last letter from current guess
         */
        deleteLetter() {
            if (this.currentGuess.length > 0) {
                this.currentGuess = this.currentGuess.slice(0, -1);
                this.updateDisplay();
            }
        },

        /**
         * Update the visual display of the current guess
         */
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

        /**
         * Sync client state with server game state after HTMX updates
         */
        updateGameState() {
            const board = document.getElementById('game-board');
            if (!board) return;

            // Clear current guess after any submission
            this.currentGuess = '';

            // Check if game has ended
            this.gameOver = board.querySelector('.game-over-container') !== null;

            // Find the current active row
            const rows = board.querySelectorAll('.guess-row');
            let foundCurrentRow = false;

            rows.forEach((row, index) => {
                const tiles = row.querySelectorAll('.tile');
                const hasContent = Array.from(tiles).some(tile => tile.textContent.trim() !== '');
                const activeTiles = row.querySelectorAll('.tile.active');

                // First empty row or row with active tiles
                if (!hasContent && !foundCurrentRow && !this.gameOver) {
                    this.currentRow = index;
                    foundCurrentRow = true;
                } else if (activeTiles.length > 0 && !foundCurrentRow) {
                    this.currentRow = index;
                    foundCurrentRow = true;
                }
            });

            if (!foundCurrentRow && !this.gameOver) {
                this.currentRow = Math.min(5, rows.length);
            }

            // Update UI components
            this.updateKeyboardColors();
            this.animateNewGuess();
            this.checkForWin();
        },

        /**
         * Submit the current 5-letter guess to the server
         */
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

        /**
         * Update keyboard key colors based on guessed letters
         */
        updateKeyboardColors() {
            const tiles = document.querySelectorAll('.tile.filled');
            this.keyStatus = {};

            tiles.forEach(tile => {
                // Skip invalid tiles - they don't affect keyboard colors
                if (tile.classList.contains('invalid')) return;
                
                const letter = tile.textContent;
                const status = tile.classList.contains('correct') ? 'correct' :
                               tile.classList.contains('present') ? 'present' :
                               tile.classList.contains('absent') ? 'absent' : '';

                if (letter && status) {
                    // Prioritize better statuses: correct > present > absent
                    if (!this.keyStatus[letter] ||
                        status === 'correct' ||
                        (status === 'present' && this.keyStatus[letter] === 'absent')) {
                        this.keyStatus[letter] = status;
                    }
                }
            });
        },

        /**
         * Get CSS class for keyboard key based on its status
         */
        getKeyClass(letter) {
            return this.keyStatus[letter] || '';
        },

        /**
         * Animate the tile flip effect for newly submitted guesses
         */
        animateNewGuess() {
            const rows = document.querySelectorAll('.guess-row');
            const lastFilledRow = Array.from(rows).find((row, index) => {
                const filledTiles = row.querySelectorAll('.tile.filled');
                const invalidTiles = row.querySelectorAll('.tile.invalid');
                
                // Only animate valid guesses (not invalid dictionary words)
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

        /**
         * Check for winning condition and add celebration animation
         */
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

        /**
         * Generate and copy share results to clipboard
         */
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

        /**
         * Copy text to clipboard with fallback for older browsers
         */
        async copyToClipboard(text) {
            try {
                await navigator.clipboard.writeText(text);
                alert("Results copied to clipboard!");
            } catch (err) {
                // Fallback for older browsers
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

// Global function for share button in template
window.shareResults = function() {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};