const WORD_LENGTH = 5;
const MAX_GUESSES = 6;
const ANIMATION_DELAY = 100;
const TOAST_DURATION = 2000;
const LETTER_REGEX = /^[a-zA-Z]$/;

document.addEventListener('gesturestart', (e) => e.preventDefault());
let lastTouchEnd = 0;
document.addEventListener(
    'touchend',
    function (event) {
        const now = Date.now();
        if (now - lastTouchEnd <= 300) {
            event.preventDefault();
        }
        lastTouchEnd = now;
    },
    false
);

function debounce(func, wait) {
    let timeout;
    return function (...args) {
        clearTimeout(timeout);
        timeout = setTimeout(() => func.apply(this, args), wait);
    };
}

function readCookie(name) {
    const v = `; ${document.cookie}`;
    const parts = v.split(`; ${name}=`);
    if (parts.length === 2) {
        return parts.pop().split(';').shift();
    }
    return '';
}

window.gameApp = function () {
    return {
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: {},
        showCopyModal: false,
        copyModalText: '',
        showToast: false,
        toastMessage: '',
        toastType: 'info',
        submittingGuess: false,
        _gameRows: null,
        _guessRows: null,
        getGameRows() {
            if (!this._gameRows) {
                this._gameRows = document.querySelectorAll('#game-board > div');
            }
            return this._gameRows;
        },
        getGuessRows() {
            if (!this._guessRows) {
                this._guessRows = document.querySelectorAll('.guess-row');
            }
            return this._guessRows;
        },
        clearDOMCache() {
            this._gameRows = null;
            this._guessRows = null;
        },
        initGame() {
            this.resetGameState();
            this.initTheme();
            this.setupHTMXHandlers();
            setTimeout(() => this.updateGameState(), 100);
        },
        resetGameState() {
            this.currentGuess = '';
            this.currentRow = 0;
            this.gameOver = false;
            this.keyStatus = {};
            this.submittingGuess = false;
            this.clearDOMCache();
        },
        initTheme() {
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        },
        setupHTMXHandlers() {
            document.body.addEventListener('htmx:afterSwap', (evt) => {
                this.submittingGuess = false;
                this.clearDOMCache();
                this.restoreUserInput();


                const targetEl = evt?.detail?.target || document.getElementById('game-board-container');
                if (window.Alpine && targetEl) {
                    if (typeof window.Alpine.initTree === 'function') {
                        window.Alpine.initTree(targetEl);
                    } else if (typeof window.Alpine.discoverUninitializedComponents === 'function') {
                        window.Alpine.discoverUninitializedComponents((el) => {
                            window.Alpine.initializeComponent(el);
                        }, targetEl);
                    }
                }

                const errorFlag = document.getElementById('guess-error-flag');
                if (errorFlag) {
                    const msg = errorFlag.getAttribute('data-message') || 'Word not accepted. Try another word.';
                    this.showToastNotification(msg, 'warning');
                    this.shakeCurrentRow();
                } else if (document.getElementById('not-accepted-flag')) {
                    this.showToastNotification(
                        'Word not in accepted list. Try another word.',
                        'warning'
                    );
                    this.shakeCurrentRow();
                } else {
                    this.updateGameState();
                }
            });

            document.body.addEventListener('htmx:beforeSwap', () => {
                if (this.currentGuess) {
                    this.tempCurrentGuess = this.currentGuess;
                    this.tempCurrentRow = this.currentRow;
                }
            });

            document.body.addEventListener('htmx:responseError', (evt) => {
                if (evt.detail.xhr.status === 429) {
                    this.showToastNotification(
                        'Too many requests. Please slow down! ⏰',
                        'warning'
                    );
                } else {
                    this.showToastNotification(
                        'Connection error. Please try again! 🔄',
                        'error'
                    );
                }
            });

            document.body.addEventListener('htmx:sendError', () => {
                this.showToastNotification(
                    'Network error. Check your connection! 📡',
                    'error'
                );
            });

            document.body.addEventListener('htmx:timeout', () => {
                this.showToastNotification(
                    'Request timed out. Please try again! ⏱️',
                    'error'
                );
            });

            document.body.addEventListener('htmx:afterSettle', (evt) => {
                const triggerHeader = evt.detail.xhr.getResponseHeader('HX-Trigger');
                if (triggerHeader && triggerHeader.includes('clear-completed-words')) {
                    this.clearCompletedWords();
                }
            });

            if (window.htmx) {
                htmx.on('htmx:configRequest', (evt) => {
                    let token = readCookie('csrf_token');

                    if (!token) {
                        const meta = document.querySelector('meta[name="csrf-token"]');
                        token = meta ? meta.getAttribute('content') : '';
                    }

                    if (token) {
                        evt.detail.headers['X-CSRF-Token'] = token;
                    }
                });
            }
        },
        restoreUserInput() {
            if (this.tempCurrentGuess && !this.currentGuess) {
                this.currentGuess = this.tempCurrentGuess;
                this.currentRow = this.tempCurrentRow;
                this.updateDisplay();
                this.tempCurrentGuess = null;
                this.tempCurrentRow = null;
            } else {
                this.tempCurrentGuess = null;
                this.tempCurrentRow = null;
            }
        },
        updateDisplay: debounce(function () {
            const rows = this.getGameRows();
            const row = rows?.[this.currentRow];
            if (!row) {
                return;
            }
            const tiles = row.querySelectorAll('.tile');
            tiles?.forEach((tile, i) => {
                tile.classList.remove(
                    'tile-correct',
                    'tile-present',
                    'tile-absent'
                );
                const letter = this.currentGuess[i] || '';
                tile.textContent = letter;
                if (letter) {
                    tile.classList.add('filled');
                } else {
                    tile.classList.remove('filled');
                }
            });
        }, 50),
        shakeCurrentRow() {
            const rows = this.getGuessRows();
            const targetRow = Math.max(0, this.currentRow);
            const row = rows?.[targetRow];
            if (row) {
                row.classList.add('shake');
                setTimeout(() => row.classList.remove('shake'), 500);
            }
        },
        handleKeyPress(e) {
            if (this.gameOver) {
                if (
                    e.key === 'Enter' ||
                    LETTER_REGEX.test(e.key) ||
                    e.key === 'Backspace'
                ) {
                    this.showToastNotification(
                        'Game is over! Start a new game to continue! 🎮',
                        'warning'
                    );
                }
                return;
            }

            if (e.key === 'Enter') {
                this.submitGuess();
            } else if (e.key === 'Backspace') {
                this.deleteLetter();
            } else if (LETTER_REGEX.test(e.key)) {
                this.addLetter(e.key.toUpperCase());
            }
        },
        handleVirtualKey(key, evt) {
            if (this.gameOver) {
                this.showToastNotification(
                    'Game is over! Start a new game to continue! 🎮',
                    'warning'
                );
                return;
            }

            const active = evt?.target;
            if (active && active.disabled !== undefined) {
                active.disabled = true;
                active.classList.add('pressed');
                setTimeout(() => {
                    active.disabled = false;
                    active.classList.remove('pressed');
                }, 120);
            }

            if (key === 'ENTER') {
                this.submitGuess();
            } else if (key === 'BACKSPACE') {
                this.deleteLetter();
            } else {
                this.addLetter(key);
            }
        },
        addLetter(letter) {
            if (this.currentGuess.length < WORD_LENGTH) {
                this.currentGuess += letter;
                this.updateDisplay();
            } else {
                this.showToastNotification(
                    `Word is already ${WORD_LENGTH} letters! Press Enter to submit! ⌨️`,
                    'warning'
                );
                this.shakeCurrentRow();
            }
        },
        deleteLetter() {
            if (this.currentGuess.length > 0) {
                this.currentGuess = this.currentGuess.slice(0, -1);
                this.updateDisplay();
            }
        },
        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-bs-theme', theme);
            localStorage.setItem('theme', theme);
        },
        updateGameState() {
            const board = document.getElementById('game-board');
            if (!board) {
                return;
            }

            if (!document.getElementById('guess-error-flag')) {
                this.currentGuess = '';
            }

            this.keyStatus = {};

            const gameOverContainer = board.parentElement.querySelector('.mt-3.p-3.bg-body-secondary');
            this.gameOver = gameOverContainer !== null;

            const rows = this.getGuessRows();
            let completedRows = 0;
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                const hasStatusTiles = Array.from(tiles).some(
                    (tile) =>
                        tile.classList.contains('tile-correct') ||
                        tile.classList.contains('tile-present') ||
                        tile.classList.contains('tile-absent')
                );
                if (hasStatusTiles) {
                    completedRows++;
                }
            });

            this.currentRow = Math.min(completedRows, rows.length - 1);
            this.updateKeyboardColors();
            this.animateNewGuess();
            this.checkForWin();
        },

        submitGuess() {
            if (
                this.submittingGuess ||
                this.gameOver ||
                this.currentGuess.length !== WORD_LENGTH
            ) {
                if (this.currentGuess.length < WORD_LENGTH) {
                    this.showToastNotification(
                        `Word must be ${WORD_LENGTH} letters long! ✏️`,
                        'warning'
                    );
                    this.shakeCurrentRow();
                } else if (this.gameOver) {
                    this.showToastNotification(
                        'Game is already over! Start a new game! 🎮',
                        'warning'
                    );
                    this.shakeCurrentRow();
                }
                return;
            }

            this.submittingGuess = true;
            const guessInput = document.getElementById('guess-input');
            guessInput.value = this.currentGuess;

            htmx.trigger('#guess-form', 'submit');
        },
        updateKeyboardColors() {
            const tiles = document.querySelectorAll('.tile.filled');
            this.keyStatus = {};
            tiles.forEach((tile) => {
                const letter = tile.textContent;
                const status = tile.classList.contains('tile-correct')
                    ? 'correct'
                    : tile.classList.contains('tile-present')
                        ? 'present'
                        : tile.classList.contains('tile-absent')
                            ? 'absent'
                            : '';
                if (letter && status) {
                    if (
                        !this.keyStatus[letter] ||
                        status === 'correct' ||
                        (status === 'present' &&
                            this.keyStatus[letter] === 'absent')
                    ) {
                        this.keyStatus[letter] = status;
                    }
                }
            });
        },
        getKeyClass(letter) {
            return this.keyStatus[letter] ?? '';
        },
        animateNewGuess() {
            const rows = this.getGameRows();
            const row = rows?.[this.currentRow - 1];
            if (!row || row.classList.contains('animated')) {
                return;
            }
            const tiles = row.querySelectorAll('.tile.filled');
            if (tiles.length !== WORD_LENGTH) {
                return;
            }
            tiles?.forEach((tile, index) => {
                tile.style.setProperty('--tile-index', index);
                setTimeout(() => {
                    tile.classList.add('flip');
                    setTimeout(() => {
                        tile.classList.add('flip-revealed');
                    }, 300);
                }, index * ANIMATION_DELAY);
            });
            row.classList.add('animated');
            row.classList.remove('submitting');

            setTimeout(() => {
                const sr = document.getElementById('sr-live');
                if (sr) {
                    const tiles = row.querySelectorAll('.tile.filled');
                    if (tiles.length === WORD_LENGTH) {
                        const parts = Array.from(tiles).map((tile) => {
                            const letter = tile.textContent || '';
                            const status = tile.classList.contains('tile-correct') ? 'correct' : (tile.classList.contains('tile-present') ? 'present' : (tile.classList.contains('tile-absent') ? 'absent' : 'unknown'));
                            return `${letter} is ${status}`;
                        });
                        sr.textContent = `Row ${this.currentRow} revealed: ${parts.join(', ')}.`;
                    }
                }
            }, WORD_LENGTH * ANIMATION_DELAY + 400);
        },
        checkForWin() {
            const rows = this.getGameRows();
            let hasWinner = false;
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile-correct');
                if (tiles.length === WORD_LENGTH) {
                    hasWinner = true;
                    if (!row.classList.contains('winner')) {
                        row.classList.add('winner');
                        tiles.forEach((tile, index) => {
                            tile.style.setProperty('--tile-index', index);
                        });
                    }
                }
            });
            if (hasWinner) {
                this.gameOver = true;
                this.launchConfetti();

                const winningRow = Array.from(rows).find(row => {
                    const tiles = row.querySelectorAll('.tile-correct');
                    return tiles.length === WORD_LENGTH;
                });

                if (winningRow) {
                    const tiles = winningRow.querySelectorAll('.tile-correct');
                    const word = Array.from(tiles).map(tile => tile.textContent).join('');
                    if (word && word.length === WORD_LENGTH) {
                        this.saveCompletedWord(word.toUpperCase());
                    }
                } else {
                    const gameBoard = document.getElementById('game-board');
                    if (gameBoard) {
                        const gameOverContainer = gameBoard.parentElement.querySelector('.mt-3.p-3.bg-body-secondary');
                        if (gameOverContainer) {
                            const gameOverText = gameOverContainer.textContent || '';
                            const wordMatch = gameOverText.match(/word was:\s*(\w+)/i);
                            if (wordMatch && wordMatch[1]) {
                                this.saveCompletedWord(wordMatch[1].toUpperCase());
                            }
                        }
                    }
                }
            } else {
                const completedRows = Array.from(rows).filter((row) => {
                    const tiles = row.querySelectorAll('.tile.filled');
                    return (
                        tiles.length === WORD_LENGTH &&
                        Array.from(tiles).some(
                            (tile) =>
                                tile.classList.contains('tile-correct') ||
                                tile.classList.contains('tile-present') ||
                                tile.classList.contains('tile-absent')
                        )
                    );
                });

                if (completedRows.length === MAX_GUESSES && !hasWinner) {
                    setTimeout(() => {
                        this.showToastNotification(
                            'Game over! Better luck next time! 🎯',
                            'info'
                        );
                    }, 1000);
                }
            }
        },
        launchConfetti() {
            if (!window._confettiScriptLoaded && typeof window.confetti !== 'function') {
                window._confettiScriptLoaded = true;
                const script = document.createElement('script');
                script.src = 'https://cdn.jsdelivr.net/npm/canvas-confetti@1.9.3/dist/confetti.browser.min.js';
                script.onload = () => {
                    this._doConfetti();
                    this._doFireworks();
                };
                document.body.appendChild(script);
            } else if (typeof window.confetti === 'function') {
                this._doConfetti();
                this._doFireworks();
            }
        },
        _doConfetti() {
            window.confetti({
                particleCount: 120,
                spread: 80,
                origin: { y: 0.6 },
            });
        },
        _doFireworks() {
            const colors = [
                '#ff0000',
                '#00ff00',
                '#0000ff',
                '#ffff00',
                '#ff00ff',
                '#00ffff',
                '#ffa500',
                '#ff69b4',
            ];

            for (let i = 0; i < 3; i++) {
                setTimeout(() => {
                    const x = Math.random() * 0.8 + 0.1;
                    const y = Math.random() * 0.4 + 0.2;

                    window.confetti({
                        particleCount: 50,
                        angle: Math.random() * 360,
                        spread: 360,
                        startVelocity: 15,
                        decay: 0.95,
                        gravity: 0.8,
                        colors: [
                            colors[Math.floor(Math.random() * colors.length)],
                        ],
                        origin: { x: x, y: y },
                        shapes: ['circle'],
                        scalar: 0.8,
                    });
                }, i * 400);
            }

            setTimeout(() => {
                window.confetti({
                    particleCount: 100,
                    spread: 120,
                    startVelocity: 25,
                    colors: ['#ffd700', '#ffff00', '#ffffff'],
                    origin: { y: 0.4 },
                    shapes: ['circle'],
                    scalar: 0.6,
                });
            }, 1000);
        },
        shareResults() {
            const rows = this.getGameRows();
            let emojiGrid = 'Vortludo ';
            let completedRowCount = 0;
            let hasWon = false;

            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                if (tiles.length === WORD_LENGTH) {
                    completedRowCount++;
                    const correctTiles = row.querySelectorAll('.tile-correct');
                    if (correctTiles.length === WORD_LENGTH) {
                        hasWon = true;
                    }
                }
            });

            if (hasWon) {
                emojiGrid += `${completedRowCount}/6\n\n`;
            } else {
                emojiGrid += 'X/6\n\n';
            }

            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                if (tiles.length === WORD_LENGTH) {
                    tiles.forEach((tile) => {
                        if (tile.classList.contains('tile-correct')) {
                            emojiGrid += '🟩';
                        }
                        else if (tile.classList.contains('tile-present')) {
                            emojiGrid += '🟨';
                        }
                        else {
                            emojiGrid += '⬛';
                        }
                    });
                    emojiGrid += '\n';
                }
            });
            this.copyToClipboard(emojiGrid.trim());
        },
        async copyToClipboard(text) {
            try {
                if (navigator.clipboard && window.isSecureContext) {
                    await navigator.clipboard.writeText(text);
                    this.showToastNotification('Results copied to clipboard!', 'success');
                    return;
                }
                this.openCopyModal(text);
            } catch (err) {
                this.openCopyModal(text);
                this.showToastNotification('Could not copy to clipboard automatically. Please copy manually.', 'warning');
            }
        },
        openCopyModal(text) {
            this.copyModalText = text;
            const modalEl = document.querySelector('.modal');
            if (window.bootstrap && modalEl) {
                if (!this._bsModal) {
                    this._bsModal = new bootstrap.Modal(modalEl);
                }
                this._bsModal.show();
            } else {
                this.showCopyModal = true;
            }
        },
        closeCopyModal() {
            const modalEl = document.querySelector('.modal');
            if (window.bootstrap && modalEl && this._bsModal) {
                this._bsModal.hide();
            } else {
                this.showCopyModal = false;
            }
            this.copyModalText = '';
        },
        selectAllText() {
            const textarea = document.querySelector('.copy-modal textarea');
            if (textarea) {
                textarea.select();
                textarea.focus();
            } else {
                this.showToastNotification('Could not select text for copying.', 'warning');
            }
        },
        showToastNotification(message, type = 'success') {
            this.toastMessage = message;
            this.toastType = type;

            this.$nextTick(() => {
                const toastElement = document.getElementById('notification-toast');
                if (toastElement && typeof bootstrap !== 'undefined' && bootstrap.Toast) {
                    const toast = new bootstrap.Toast(toastElement, { delay: 3000 });
                    toast.show();
                }
            });
        },
        shouldHideKeyboard() {
            return this.gameOver;
        },
        getCompletedWords() {
            try {
                const completed = localStorage.getItem('vortludo-completed-words');
                return completed ? JSON.parse(completed) : [];
            } catch {
                this.showToastNotification('Could not load completed words from your browser storage.', 'warning');
                return [];
            }
        },
        saveCompletedWord(word) {
            try {
                const completed = this.getCompletedWords();
                if (!completed.includes(word)) {
                    completed.push(word);
                    localStorage.setItem('vortludo-completed-words', JSON.stringify(completed));
                    this.showToastNotification(`Word "${word}" added to your completed list! 🎯`, 'success');
                }
            } catch {
                this.showToastNotification('Could not save completed word to your browser storage.', 'warning');
            }
        },
        clearCompletedWords() {
            try {
                localStorage.removeItem('vortludo-completed-words');
                this.showToastNotification('🎉 Congratulations! You\'ve completed all words! Progress reset.', 'success');
            } catch {
                this.showToastNotification('Could not clear completed words from your browser storage.', 'warning');
            }
        },
        startNewGame() {
            const completedWords = this.getCompletedWords();
            const form = document.createElement('form');
            form.method = 'POST';
            form.action = '/new-game';
            form.style.display = 'none';

            const token = readCookie('csrf_token');
            if (token) {
                const csrfInput = document.createElement('input');
                csrfInput.type = 'hidden';
                csrfInput.name = 'csrf_token';
                csrfInput.value = token;
                form.appendChild(csrfInput);
            }

            if (completedWords.length > 0) {
                const input = document.createElement('input');
                input.type = 'hidden';
                input.name = 'completedWords';
                input.value = JSON.stringify(completedWords);
                form.appendChild(input);
            }

            document.body.appendChild(form);
            form.submit();
        },
    };
};

window.shareResults = function () {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};
