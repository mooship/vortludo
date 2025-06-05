/**
 * HTMX Event Handlers for Game Feedback
 * Manages server responses and client-side visual feedback
 */

// Handle invalid word submissions with shake animation and auto-dismiss alerts
document.body.addEventListener('htmx:afterSwap', function(evt) {
    const errorAlert = document.querySelector('.alert-danger');
    if (errorAlert && (errorAlert.textContent.includes('Not in word list') || errorAlert.textContent.includes('Word must be 5 letters'))) {
        // Auto-dismiss error alerts after 3 seconds for better UX
        setTimeout(() => {
            if (errorAlert && errorAlert.parentNode) {
                const bsAlert = bootstrap.Alert.getOrCreateInstance(errorAlert);
                bsAlert.close();
            }
        }, 3000);

        // Apply shake animation to the current guess row for visual feedback
        if (window.Alpine) {
            const alpineData = Alpine.$data(document.querySelector('[x-data]'));
            if (alpineData) {
                const rows = document.querySelectorAll('.d-flex.justify-content-center.mb-1');
                // Target the previous row since server already incremented currentRow
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

// Preserve user input state before HTMX processes server response
document.body.addEventListener('htmx:beforeSwap', function(evt) {
    if (window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData && alpineData.currentGuess) {
            // Store current input temporarily for restoration after invalid guesses
            window.tempCurrentGuess = alpineData.currentGuess;
            window.tempCurrentRow = alpineData.currentRow;
        }
    }
});

// Restore user input after invalid guess responses (preserves typing state)
document.body.addEventListener('htmx:afterSwap', function(evt) {
    if (window.tempCurrentGuess && window.Alpine) {
        const alpineData = Alpine.$data(document.querySelector('[x-data]'));
        if (alpineData) {
            alpineData.currentGuess = window.tempCurrentGuess;
            alpineData.currentRow = window.tempCurrentRow;
            alpineData.updateDisplay();
        }
        // Clean up temporary storage
        window.tempCurrentGuess = null;
        window.tempCurrentRow = null;
    }
});

/**
 * Mobile Touch Event Handlers
 * Prevents common mobile web app issues like zoom gestures
 */

// Prevent pinch-zoom gestures on mobile devices for app-like experience
document.addEventListener('gesturestart', function(e) {
    e.preventDefault();
});

// Prevent double-tap zoom while allowing single taps
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

        /**
         * Initialize game on page load
         * Sets up theme and syncs with server state
         */
        initGame() {
            // Clear any cached game state
            this.currentGuess = '';
            this.currentRow = 0;
            this.gameOver = false;
            this.keyStatus = {};
            
            // Load and apply saved theme preference from localStorage
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);

            // Force a small delay before syncing to ensure DOM is ready
            setTimeout(() => {
                this.updateGameState();
            }, 100);
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

            if (e.key === 'Enter') {
                this.submitGuess();
            } else if (e.key === 'Backspace') {
                this.deleteLetter();
            } else if (/^[a-zA-Z]$/.test(e.key)) {
                this.addLetter(e.key.toUpperCase());
            }
        },

        /**
         * Handle virtual on-screen keyboard clicks
         * Processes special keys (ENTER, BACKSPACE) and letters
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
            const rows = document.querySelectorAll('#game-board > div');
            const row = rows[this.currentRow];
            if (!row) return;
            
            const tiles = row.querySelectorAll('.tile');
            tiles.forEach((tile, i) => {
                const letter = this.currentGuess[i] || '';
                tile.textContent = letter;
                // Add/remove 'filled' class for visual styling
                if (letter) {
                    tile.classList.add('filled');
                } else {
                    tile.classList.remove('filled');
                }
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
            const gameOverContainer = board.parentElement.querySelector('.mt-3.p-3.bg-light');
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
                if (hasStatusTiles) {
                    completedRows++;
                }
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
                
                // Only animate valid dictionary words
                return filledTiles.length === 5 &&
                       !row.classList.contains('animated') &&
                       (row.classList.contains('submitting') || index < this.currentRow);
            });

            if (lastFilledRow) {
                const tiles = lastFilledRow.querySelectorAll('.tile.filled');
                tiles.forEach((tile, index) => {
                    const letter = tile.textContent;
                    tile.style.setProperty('--tile-index', index);

                    // Stagger flip animation across tiles for visual appeal
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
         * Detect winning condition and add celebration animation
         * Emits custom event when player wins for other components to react
         */
        checkForWin() {
            const rows = document.querySelectorAll('#game-board > div');
            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile-correct');
                if (tiles.length === 5 && !row.classList.contains('winner')) {
                    row.classList.add('winner');
                    tiles.forEach((tile, index) => {
                        tile.style.setProperty('--tile-index', index);
                    });
                    
                    // Notify other components that game was won
                    window.dispatchEvent(new CustomEvent('gameWon'));
                }
            });
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
                        if (tile.classList.contains('tile-correct')) {
                            emojiGrid += '🟩';
                        } else if (tile.classList.contains('tile-present')) {
                            emojiGrid += '🟨';
                        } else {
                            emojiGrid += '⬛';
                        }
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
                    this.showCopyNotification("Results copied to clipboard!");
                    return;
                }
                
                // Fallback for non-secure contexts or older browsers
                this.showCopyDialog(text);
                
            } catch (err) {
                console.error('Clipboard copy failed:', err);
                this.showCopyDialog(text);
            }
        },

        /**
         * Show modal dialog with text to copy manually when clipboard API unavailable
         * Provides accessible fallback for all environments
         */
        showCopyDialog(text) {
            // Remove any existing dialogs to prevent duplicates
            const existingDialog = document.querySelector('.modal.copy-dialog');
            if (existingDialog) {
                bootstrap.Modal.getInstance(existingDialog)?.dispose();
                existingDialog.remove();
            }

            // Create Bootstrap modal with copy-friendly text area
            const dialogHTML = `
                <div class="modal fade copy-dialog" tabindex="-1" aria-hidden="true">
                    <div class="modal-dialog modal-dialog-centered">
                        <div class="modal-content">
                            <div class="modal-header">
                                <h5 class="modal-title">Copy Results</h5>
                                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                            </div>
                            <div class="modal-body">
                                <p>Please copy the text below:</p>
                                <textarea class="form-control font-monospace" rows="8" readonly>${text}</textarea>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
                                <button type="button" class="btn btn-primary" onclick="this.closest('.modal-body').querySelector('textarea').select(); this.closest('.modal-body').querySelector('textarea').focus();">Select All</button>
                            </div>
                        </div>
                    </div>
                </div>
            `;

            // Add modal to DOM and display
            document.body.insertAdjacentHTML('beforeend', dialogHTML);
            const modalElement = document.querySelector('.copy-dialog');
            const modal = new bootstrap.Modal(modalElement);
            modal.show();

            // Auto-select text when modal opens for easy copying
            modalElement.addEventListener('shown.bs.modal', function() {
                const textarea = this.querySelector('textarea');
                textarea.select();
                textarea.focus();
            });

            // Clean up modal element when closed
            modalElement.addEventListener('hidden.bs.modal', function() {
                bootstrap.Modal.getInstance(this)?.dispose();
                this.remove();
            });
        },

        /**
         * Display temporary floating notification for copy operations
         * Shows success/error feedback with auto-dismiss
         */
        showCopyNotification(message, isError = false) {
            // Create Bootstrap toast instead of custom alert
            const toastHTML = `
                <div class="toast-container position-fixed top-0 start-50 translate-middle-x p-3">
                    <div class="toast align-items-center text-bg-${isError ? 'danger' : 'primary'}" role="alert" aria-live="assertive" aria-atomic="true">
                        <div class="d-flex">
                            <div class="toast-body">
                                ${message}
                            </div>
                            <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast" aria-label="Close"></button>
                        </div>
                    </div>
                </div>
            `;

            // Remove any existing toasts
            document.querySelectorAll('.toast-container').forEach(el => el.remove());

            // Add to DOM
            document.body.insertAdjacentHTML('beforeend', toastHTML);
            
            // Show toast
            const toastElement = document.querySelector('.toast');
            const toast = new bootstrap.Toast(toastElement, { delay: 3000 });
            toast.show();

            // Clean up after hide
            toastElement.addEventListener('hidden.bs.toast', function() {
                this.closest('.toast-container').remove();
            });
        }
    };
}

// Hide virtual keyboard when game is won
window.addEventListener('gameWon', function() {
    const keyboard = document.querySelector('.keyboard');
    if (keyboard) {
        keyboard.classList.add('d-none');
    }
});

// Global function accessible from HTML template share button
window.shareResults = function() {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};