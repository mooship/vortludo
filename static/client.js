/**
 * HTMX Event Handlers for Game Feedback
 * Manages server responses and client-side visual feedback
 */

/**
 * Mobile Touch Event Handlers
 * Prevents common mobile web app issues like zoom gestures
 */

// Prevent pinch-zoom gestures on mobile devices for app-like experience
document.addEventListener('gesturestart', e => e.preventDefault());

// Prevent double-tap zoom while allowing single taps
let lastTouchEnd = 0;
document.addEventListener('touchend', function (event) {
    const now = Date.now();
    if (now - lastTouchEnd <= 300) {
        event.preventDefault();
    }
    lastTouchEnd = now;
}, false);

/**
 * Main Game Application using Alpine.js
 * Manages all client-side game state and user interactions
 */
window.gameApp = function() {
    return {
        // Core game state properties
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: {}, // Tracks keyboard key colors based on guessed letters
        showCopyModal: false,
        copyModalText: '',
        showToast: false,
        toastMessage: '',
        toastType: 'primary',
        
        // Constants
        WORD_LENGTH: 5,
        MAX_GUESSES: 6,
        ANIMATION_DELAY: 100,
        TOAST_DURATION: 3000,

        /**
         * Initialize game on page load
         * Sets up theme, event listeners, and syncs with server state
         */
        initGame() {
            this.resetGameState();
            this.initTheme();
            this.setupHTMXHandlers();
            
            // Force a small delay before syncing to ensure DOM is ready
            setTimeout(() => this.updateGameState(), 100);
        },
        
        /**
         * Reset game state to initial values
         */
        resetGameState() {
            this.currentGuess = '';
            this.currentRow = 0;
            this.gameOver = false;
            this.keyStatus = {};
        },
        
        /**
         * Initialize theme from localStorage
         */
        initTheme() {
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        },

        /**
         * Setup HTMX event handlers using Alpine context
         */
        setupHTMXHandlers() {
            document.body.addEventListener('htmx:afterSwap', (evt) => {
                this.handleErrorAlerts();
                this.restoreUserInput();
                this.updateGameState();
            });

            document.body.addEventListener('htmx:beforeSwap', (evt) => {
                if (this.currentGuess) {
                    this.tempCurrentGuess = this.currentGuess;
                    this.tempCurrentRow = this.currentRow;
                }
            });
        },
        
        /**
         * Handle error alerts with auto-dismiss
         */
        handleErrorAlerts() {
            const errorAlert = document.querySelector('.alert-danger');
            if (!errorAlert) return;
            
            const isInvalidWordError = 
                errorAlert.textContent.includes('Not in word list') || 
                errorAlert.textContent.includes('Word must be 5 letters');
                
            if (isInvalidWordError) {
                setTimeout(() => {
                    if (errorAlert?.parentNode) {
                        bootstrap.Alert.getOrCreateInstance(errorAlert).close();
                    }
                }, this.TOAST_DURATION);
                this.shakeCurrentRow();
            }
        },
        
        /**
         * Restore user input after server response
         */
        restoreUserInput() {
            if (this.tempCurrentGuess) {
                this.currentGuess = this.tempCurrentGuess;
                this.currentRow = this.tempCurrentRow;
                this.updateDisplay();
                this.tempCurrentGuess = null;
                this.tempCurrentRow = null;
            }
        },

        /**
         * Apply shake animation to current guess row
         */
        shakeCurrentRow() {
            const rows = document.querySelectorAll('.d-flex.justify-content-center.mb-1');
            // Target the previous row since server already incremented currentRow
            const targetRow = Math.max(0, this.currentRow - 1);
            if (rows[targetRow]) {
                rows[targetRow].classList.add('shake');
                setTimeout(() => rows[targetRow].classList.remove('shake'), 500);
            }
        },

        /**
         * Toggle between light and dark themes using Bootstrap 5.3 color modes
         * Persists preference in localStorage
         */
        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-bs-theme', theme);
            localStorage.setItem('theme', theme);
        },

        /**
         * Handle physical keyboard input events
         * Processes Enter, Backspace, and letter keys
         */
        handleKeyPress(e) {
            if (this.gameOver) return;
            
            if (e.key === 'Enter') this.submitGuess();
            else if (e.key === 'Backspace') this.deleteLetter();
            else if (/^[a-zA-Z]$/.test(e.key)) this.addLetter(e.key.toUpperCase());
        },

        /**
         * Handle virtual on-screen keyboard clicks
         * Processes special keys (ENTER, BACKSPACE) and letters
         */
        handleVirtualKey(key) {
            if (this.gameOver) return;

            if (key === 'ENTER') this.submitGuess();
            else if (key === 'BACKSPACE') this.deleteLetter();
            else this.addLetter(key);
        },

        /**
         * Add a letter to the current guess (max 5 letters)
         * Updates display immediately for responsive feedback
         */
        addLetter(letter) {
            if (this.currentGuess.length < 5) {
                this.currentGuess += letter;
                this.updateDisplay();
            }
        },

        /**
         * Remove the last letter from current guess
         * Updates display immediately for responsive feedback
         */
        deleteLetter() {
            if (this.currentGuess.length > 0) {
                this.currentGuess = this.currentGuess.slice(0, -1);
                this.updateDisplay();
            }
        },

        /**
         * Update visual display of current guess in game grid
         * Shows letters as user types before submission
         */
        updateDisplay() {
            const row = document.querySelectorAll('#game-board > div')[this.currentRow];
            if (!row) return;
            
            row.querySelectorAll('.tile').forEach((tile, i) => {
                const letter = this.currentGuess[i] || '';
                tile.textContent = letter;
                tile.classList.toggle('filled', Boolean(letter));
            });
        },

        /**
         * Synchronize client state with server game state after HTMX updates
         * Critical for maintaining consistency between client UI and server logic
         */
        updateGameState() {
            const board = document.getElementById('game-board');
            if (!board) return;

            // Always reset current guess after any server interaction
            this.currentGuess = '';
            this.keyStatus = {};

            // Check if game has ended by looking for the game over container
            const gameOverContainer = board.parentElement.querySelector('.mt-3.p-3.bg-body-secondary');
            this.gameOver = gameOverContainer !== null;

            // Count rows that have filled tiles with status classes (completed guesses)
            const rows = document.querySelectorAll('.guess-row');
            let completedRows = 0;
            
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                // Check if this row has status tiles (correct/present/absent) - means it's completed
                const hasStatusTiles = Array.from(tiles).some(tile => 
                    tile.classList.contains('tile-correct') || 
                    tile.classList.contains('tile-present') || 
                    tile.classList.contains('tile-absent')
                );
                if (hasStatusTiles) completedRows++;
            });

            // Current row should be the next empty row after completed ones
            this.currentRow = Math.min(completedRows, rows.length - 1);

            // Update UI components based on new state
            this.updateKeyboardColors();
            this.animateNewGuess();
            this.checkForWin();
        },

        /**
         * Submit current 5-letter guess to server via HTMX
         * Only submits complete 5-letter words
         */
        submitGuess() {
            if (this.currentGuess.length === 5) {
                // Add visual feedback during submission
                const rows = document.querySelectorAll('.guess-row');
                if (rows[this.currentRow]) {
                    rows[this.currentRow].classList.add('submitting');
                }

                // Trigger HTMX form submission
                document.getElementById('guess-input').value = this.currentGuess;
                document.getElementById('guess-form').dispatchEvent(new Event('submit'));
            }
        },

        /**
         * Update virtual keyboard key colors based on letter status in completed guesses
         * Follows Wordle color priority: correct (green) > present (yellow) > absent (gray)
         */
        updateKeyboardColors() {
            const tiles = document.querySelectorAll('.tile.filled');
            this.keyStatus = {};

            tiles.forEach(tile => {
                const letter = tile.textContent;
                const status = tile.classList.contains('tile-correct') ? 'correct' :
                               tile.classList.contains('tile-present') ? 'present' :
                               tile.classList.contains('tile-absent') ? 'absent' : '';

                if (letter && status) {
                    // Apply color priority logic: correct overrides all, present overrides absent
                    if (!this.keyStatus[letter] ||
                        status === 'correct' ||
                        (status === 'present' && this.keyStatus[letter] === 'absent')) {
                        this.keyStatus[letter] = status;
                    }
                }
            });
        },

        /**
         * Get CSS class for virtual keyboard key based on letter status
         * Returns empty string for unguessed letters
         */
        getKeyClass(letter) {
            return this.keyStatus[letter] || '';
        },

        /**
         * Animate tile flip effect for newly submitted valid guesses
         * Skips animation for invalid words to provide clear feedback distinction
         */
        animateNewGuess() {
            const rows = document.querySelectorAll('#game-board > div');
            const lastFilledRow = Array.from(rows).find((row, index) => {
                const filledTiles = row.querySelectorAll('.tile.filled');
                return filledTiles.length === this.WORD_LENGTH &&
                       !row.classList.contains('animated') &&
                       (row.classList.contains('submitting') || index < this.currentRow);
            });
            
            if (!lastFilledRow) return;
            
            const tiles = lastFilledRow.querySelectorAll('.tile.filled');
            tiles.forEach((tile, index) => {
                tile.style.setProperty('--tile-index', index);
                setTimeout(() => tile.classList.add('flip'), index * this.ANIMATION_DELAY);
            });
            
            lastFilledRow.classList.add('animated');
            lastFilledRow.classList.remove('submitting');
        },

        /**
         * Detect winning condition and add celebration animation
         * Hides virtual keyboard when game is won
         */
        checkForWin() {
            const rows = document.querySelectorAll('#game-board > div');
            let hasWinner = false;
            
            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile-correct');
                if (tiles.length === 5) {
                    hasWinner = true;
                    if (!row.classList.contains('winner')) {
                        row.classList.add('winner');
                        tiles.forEach((tile, index) => {
                            tile.style.setProperty('--tile-index', index);
                        });
                    }
                }
            });
            
            // Update game over state if we found a winner
            if (hasWinner) {
                this.gameOver = true;
            }
        },

        /**
         * Generate shareable results as emoji grid and copy to clipboard
         * Creates Wordle-style emoji pattern for social sharing
         */
        shareResults() {
            const rows = document.querySelectorAll('#game-board > div');
            let emojiGrid = 'Vortludo\n\n';

            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile.filled');
                if (tiles.length === 5) {
                    tiles.forEach(tile => {
                        if (tile.classList.contains('tile-correct')) emojiGrid += '🟩';
                        else if (tile.classList.contains('tile-present')) emojiGrid += '🟨';
                        else emojiGrid += '⬛';
                    });
                    emojiGrid += '\n';
                }
            });

            this.copyToClipboard(emojiGrid.trim());
        },

        /**
         * Copy text to clipboard with fallback for non-secure contexts
         * Uses modern Clipboard API when available, shows manual copy dialog otherwise
         */
        async copyToClipboard(text) {
            try {
                // Prefer modern clipboard API for secure contexts (HTTPS/localhost)
                if (navigator.clipboard && window.isSecureContext) {
                    await navigator.clipboard.writeText(text);
                    this.showToastNotification("Results copied to clipboard!");
                    return;
                }
                
                // Fallback for non-secure contexts or older browsers
                this.openCopyModal(text);
                
            } catch (err) {
                console.error('Clipboard copy failed:', err);
                this.openCopyModal(text);
            }
        },

        /**
         * Open copy modal with text to copy manually
         */
        openCopyModal(text) {
            this.copyModalText = text;
            this.showCopyModal = true;
        },

        /**
         * Close copy modal
         */
        closeCopyModal() {
            this.showCopyModal = false;
            this.copyModalText = '';
        },

        /**
         * Select all text in copy modal textarea
         */
        selectAllText() {
            const textarea = document.querySelector('.copy-modal textarea');
            if (textarea) {
                textarea.select();
                textarea.focus();
            }
        },

        /**
         * Display temporary toast notification
         */
        showToastNotification(message, isError = false) {
            this.toastMessage = message;
            this.toastType = isError ? 'danger' : 'primary';
            this.showToast = true;

            // Auto-hide after 3 seconds
            setTimeout(() => this.showToast = false, 3000);
        },

        /**
         * Check if virtual keyboard should be hidden
         */
        shouldHideKeyboard() {
            return this.gameOver;
        }
    };
}

// Global function accessible from HTML template share button
window.shareResults = function() {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};