/**
 * Vortludo Client-Side Game Logic
 * Handles UI interactions, animations, and state synchronization with server
 */

// Mobile gesture prevention for app-like experience
document.addEventListener('gesturestart', e => e.preventDefault());

// Prevent double-tap zoom while preserving single tap
let lastTouchEnd = 0;
document.addEventListener('touchend', function (event) {
    const now = Date.now();
    if (now - lastTouchEnd <= 300) {
        event.preventDefault();
    }
    lastTouchEnd = now;
}, false);

/**
 * Main game controller using Alpine.js
 */
window.gameApp = function() {
    return {
        // Game state
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: new Map(), // O(1) keyboard color lookups
        showCopyModal: false,
        copyModalText: '',
        showToast: false,
        toastMessage: '',
        toastType: 'primary',
        
        // Performance optimizations
        domCache: new Map(),
        animationFrameIds: new Set(),
        
        // Configuration
        WORD_LENGTH: 5,
        MAX_GUESSES: 6,
        ANIMATION_DELAY: 100,
        TOAST_DURATION: 3000,
        VALID_LETTER_REGEX: /^[A-Z]$/,

        /**
         * Initialize game on page load
         */
        initGame() {
            this.resetGameState();
            this.initTheme();
            this.setupHTMXHandlers();
            this.cacheDOMElements();
            
            // Sync with server state after DOM is ready
            requestAnimationFrame(() => this.updateGameState());
        },
        
        /**
         * Cache frequently accessed DOM elements
         */
        cacheDOMElements() {
            this.domCache.set('gameBoard', document.getElementById('game-board'));
            this.domCache.set('guessForm', document.getElementById('guess-form'));
            this.domCache.set('guessInput', document.getElementById('guess-input'));
        },
        
        /**
         * Get cached element or query and cache it
         */
        getCachedElement(key, selector = null) {
            if (!this.domCache.has(key) && selector) {
                const element = document.querySelector(selector);
                if (element) {
                    this.domCache.set(key, element);
                }
            }
            return this.domCache.get(key);
        },
        
        /**
         * Clear non-essential DOM cache after HTMX updates
         */
        clearDOMCache() {
            const essentials = ['guessForm', 'guessInput'];
            for (const [key, element] of this.domCache) {
                if (!essentials.includes(key)) {
                    this.domCache.delete(key);
                }
            }
        },
        
        /**
         * Reset game to initial state
         */
        resetGameState() {
            this.currentGuess = '';
            this.currentRow = 0;
            this.gameOver = false;
            this.keyStatus.clear();
            this.cancelAllAnimations();
        },
        
        /**
         * Cancel pending animation frames
         */
        cancelAllAnimations() {
            for (const id of this.animationFrameIds) {
                cancelAnimationFrame(id);
            }
            this.animationFrameIds.clear();
        },
        
        /**
         * Load theme preference from localStorage
         */
        initTheme() {
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        },

        /**
         * Setup HTMX response handlers
         */
        setupHTMXHandlers() {
            document.body.addEventListener('htmx:afterSwap', (evt) => {
                // Refresh cache after content update
                this.clearDOMCache();
                this.cacheDOMElements();
                
                this.handleErrorAlerts();
                this.restoreUserInput();
                this.updateGameState();
            });

            document.body.addEventListener('htmx:beforeSwap', (evt) => {
                // Save current input before server update
                if (this.currentGuess) {
                    this.tempCurrentGuess = this.currentGuess;
                    this.tempCurrentRow = this.currentRow;
                }
            });
        },
        
        /**
         * Auto-dismiss error alerts
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
         * Restore input after server response
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
         * Shake animation for invalid guesses
         */
        shakeCurrentRow() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;
            
            const rows = board.querySelectorAll('.guess-row');
            const targetRow = Math.max(0, this.currentRow - 1);
            
            if (rows[targetRow]) {
                const animId = requestAnimationFrame(() => {
                    rows[targetRow].classList.add('shake');
                    setTimeout(() => {
                        rows[targetRow].classList.remove('shake');
                        this.animationFrameIds.delete(animId);
                    }, 500);
                });
                this.animationFrameIds.add(animId);
            }
        },

        /**
         * Toggle dark/light theme
         */
        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-bs-theme', theme);
            localStorage.setItem('theme', theme);
        },

        /**
         * Handle physical keyboard input
         */
        handleKeyPress(e) {
            if (this.gameOver) return;
            
            const key = e.key.toUpperCase();
            
            if (key === 'ENTER') {
                e.preventDefault();
                this.submitGuess();
            } else if (key === 'BACKSPACE') {
                e.preventDefault();
                this.deleteLetter();
            } else if (this.VALID_LETTER_REGEX.test(key)) {
                e.preventDefault();
                this.addLetter(key);
            }
        },

        /**
         * Handle virtual keyboard clicks
         */
        handleVirtualKey(key) {
            if (this.gameOver || typeof key !== 'string') return;

            key = key.toUpperCase().trim();

            if (key === 'ENTER') {
                this.submitGuess();
            } else if (key === 'BACKSPACE') {
                this.deleteLetter();
            } else if (this.VALID_LETTER_REGEX.test(key)) {
                this.addLetter(key);
            }
        },

        /**
         * Add letter to current guess
         */
        addLetter(letter) {
            if (this.currentGuess.length < this.WORD_LENGTH && this.VALID_LETTER_REGEX.test(letter)) {
                this.currentGuess += letter;
                this.updateDisplay();
            }
        },

        /**
         * Remove last letter from guess
         */
        deleteLetter() {
            if (this.currentGuess.length > 0) {
                this.currentGuess = this.currentGuess.slice(0, -1);
                this.updateDisplay();
            }
        },

        /**
         * Update tile display with current guess
         */
        updateDisplay() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;
            
            const row = board.querySelectorAll('.guess-row')[this.currentRow];
            if (!row) return;
            
            const animId = requestAnimationFrame(() => {
                row.querySelectorAll('.tile').forEach((tile, i) => {
                    const letter = this.currentGuess[i] || '';
                    tile.textContent = letter;
                    tile.classList.toggle('filled', Boolean(letter));
                });
                this.animationFrameIds.delete(animId);
            });
            this.animationFrameIds.add(animId);
        },

        /**
         * Sync client state with server after HTMX updates
         */
        updateGameState() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;

            // Reset for new state
            this.currentGuess = '';
            this.keyStatus.clear();

            // Check game status
            const gameOverContainer = board.parentElement.querySelector('.mt-3.p-3.bg-body-secondary');
            this.gameOver = gameOverContainer !== null;

            // Count completed rows
            const rows = board.querySelectorAll('.guess-row');
            let completedRows = 0;
            
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                const hasStatusTiles = Array.from(tiles).some(tile => 
                    tile.classList.contains('tile-correct') || 
                    tile.classList.contains('tile-present') || 
                    tile.classList.contains('tile-absent')
                );
                if (hasStatusTiles) completedRows++;
            });

            this.currentRow = Math.min(completedRows, rows.length - 1);

            // Update UI
            this.updateKeyboardColors();
            this.animateNewGuess();
            this.checkForWin();
        },

        /**
         * Submit guess to server
         */
        submitGuess() {
            if (this.currentGuess.length === this.WORD_LENGTH) {
                const board = this.getCachedElement('gameBoard');
                if (!board) return;
                
                // Visual feedback
                const rows = board.querySelectorAll('.guess-row');
                if (rows[this.currentRow]) {
                    rows[this.currentRow].classList.add('submitting');
                }

                // Submit via HTMX
                const guessInput = this.getCachedElement('guessInput');
                const guessForm = this.getCachedElement('guessForm');
                
                if (guessInput && guessForm) {
                    guessInput.value = this.currentGuess;
                    guessForm.dispatchEvent(new Event('submit'));
                }
            }
        },

        /**
         * Update keyboard colors based on guessed letters
         */
        updateKeyboardColors() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;
            
            const tiles = board.querySelectorAll('.tile.filled');
            this.keyStatus.clear();

            tiles.forEach(tile => {
                const letter = tile.textContent;
                const status = tile.classList.contains('tile-correct') ? 'correct' :
                               tile.classList.contains('tile-present') ? 'present' :
                               tile.classList.contains('tile-absent') ? 'absent' : '';

                if (letter && status) {
                    const currentStatus = this.keyStatus.get(letter);
                    // Priority: correct > present > absent
                    if (!currentStatus ||
                        status === 'correct' ||
                        (status === 'present' && currentStatus === 'absent')) {
                        this.keyStatus.set(letter, status);
                    }
                }
            });
        },

        /**
         * Get keyboard key styling
         */
        getKeyClass(letter) {
            return this.keyStatus.get(letter) || '';
        },

        /**
         * Animate tile flip for new guesses
         */
        animateNewGuess() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;
            
            const rows = board.querySelectorAll('.guess-row');
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
                const animId = requestAnimationFrame(() => {
                    setTimeout(() => {
                        tile.classList.add('flip');
                        this.animationFrameIds.delete(animId);
                    }, index * this.ANIMATION_DELAY);
                });
                this.animationFrameIds.add(animId);
            });
            
            lastFilledRow.classList.add('animated');
            lastFilledRow.classList.remove('submitting');
        },

        /**
         * Check for winning condition
         */
        checkForWin() {
            const board = this.getCachedElement('gameBoard');
            if (!board) return;
            
            const rows = board.querySelectorAll('.guess-row');
            let hasWinner = false;
            
            rows.forEach(row => {
                const tiles = row.querySelectorAll('.tile-correct');
                if (tiles.length === this.WORD_LENGTH) {
                    hasWinner = true;
                    if (!row.classList.contains('winner')) {
                        const animId = requestAnimationFrame(() => {
                            row.classList.add('winner');
                            tiles.forEach((tile, index) => {
                                tile.style.setProperty('--tile-index', index);
                            });
                            this.animationFrameIds.delete(animId);
                        });
                        this.animationFrameIds.add(animId);
                    }
                }
            });
            
            if (hasWinner) {
                this.gameOver = true;
            }
        },

        /**
         * Generate shareable emoji grid
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
         * Copy text to clipboard with fallback
         */
        async copyToClipboard(text) {
            try {
                if (navigator.clipboard && window.isSecureContext) {
                    await navigator.clipboard.writeText(text);
                    this.showToastNotification("Results copied to clipboard!");
                    return;
                }
                
                // Fallback for non-secure contexts
                this.openCopyModal(text);
                
            } catch (err) {
                console.error('Clipboard copy failed:', err);
                this.openCopyModal(text);
            }
        },

        /**
         * Show manual copy modal
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
         * Select all text in textarea
         */
        selectAllText() {
            const textarea = document.querySelector('.copy-modal textarea');
            if (textarea) {
                textarea.select();
                textarea.focus();
            }
        },

        /**
         * Show temporary notification
         */
        showToastNotification(message, isError = false) {
            this.toastMessage = message;
            this.toastType = isError ? 'danger' : 'primary';
            this.showToast = true;

            setTimeout(() => this.showToast = false, 3000);
        },

        /**
         * Check if keyboard should be hidden
         */
        shouldHideKeyboard() {
            return this.gameOver;
        }
    };
}

// Global share function for HTML button
window.shareResults = function() {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    if (alpineData) {
        alpineData.shareResults();
    }
};