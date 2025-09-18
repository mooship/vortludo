const WORD_LENGTH = 5;
const MAX_GUESSES = 6;
const ANIMATION_DELAY = 100;
const COMPLETED_WORDS_KEY = 'vortludo-completed-words';

const SELECTORS = {
    GAME_BOARD: '#game-board',
    GUESS_ROW: '.guess-row',
    TILE: '.tile',
    FILLED_TILE: '.tile.filled',
    GAME_CONTENT_CONTAINER: '#game-content-container',
    CSRF_META: 'meta[name="csrf-token"]',
    GUESS_INPUT: '#guess-input',
    GUESS_FORM: '#guess-form',
    SR_LIVE: '#sr-live',
    NOTIFICATION_TOAST: '#notification-toast',
    COPY_MODAL: '.modal',
    COPY_MODAL_TEXTAREA: '.copy-modal textarea',
};

const CSS_CLASSES = {
    CORRECT: 'tile-correct',
    PRESENT: 'tile-present',
    ABSENT: 'tile-absent',
    FILLED: 'filled',
    SHAKE: 'shake',
    FLIP: 'flip',
    FLIP_REVEALED: 'flip-revealed',
    ANIMATED: 'animated',
    WINNER: 'winner',
    PRESSED: 'pressed',
};

const REGEX = {
    LETTER: /^[a-zA-Z]$/,
};

const CONFETTI_COLORS = [
    '#ff0000',
    '#00ff00',
    '#0000ff',
    '#ffff00',
    '#ff00ff',
    '#00ffff',
    '#ffa500',
    '#ff69b4',
];

document.addEventListener('gesturestart', (e) => e.preventDefault());
let lastTouchEnd = 0;
document.addEventListener(
    'touchend',
    (event) => {
        const now = Date.now();
        if (now - lastTouchEnd <= 300) {
            event.preventDefault();
        }
        lastTouchEnd = now;
    },
    false
);

document.addEventListener('click', (e) => {
    const target = e.target.closest('[data-autoblur]');
    if (target) {
        setTimeout(() => {
            target.blur?.();
        }, 60);
    }
});

const debounce = (func, wait) => {
    let timeout;
    return (...args) => {
        if (timeout) {
            clearTimeout(timeout);
        }
        timeout = setTimeout(() => func.apply(this, args), wait);
    };
};

const readCookie = (name) => {
    const v = `; ${document.cookie}`;
    const parts = v.split(`; ${name}=`);
    return parts.length === 2 ? parts.pop().split(';').shift() : '';
};

window.gameApp = function () {
    return {
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        hintVisible: false,
        isDarkMode: false,
        keyStatus: {},
        showCopyModal: false,
        copyModalText: '',
        submittingGuess: false,
        lastServerError: '',
        keepInputAfterError: false,
        _gameRows: null,
        _guessRows: null,
        _toast: null,
        _bsModal: null,
        errorCodeMessages: {
            game_over: {
                text: 'Game is already over! Start a new game! 🎮',
                type: 'warning',
            },
            invalid_length: {
                text: `Word must be ${WORD_LENGTH} letters long! ✏️`,
                type: 'warning',
            },
            no_more_guesses: {
                text: 'No more guesses allowed! Start a new game! 🚫',
                type: 'warning',
            },
            not_in_word_list: {
                text: 'Word not recognised! 📘',
                type: 'warning',
            },
            word_not_accepted: {
                text: 'Word not accepted. Try another word! 🔁',
                type: 'warning',
            },
            duplicate_guess: {
                text: 'You already guessed that word! 🔂',
                type: 'warning',
            },
            unknown_error: {
                text: 'An unexpected error occurred. ❗',
                type: 'error',
            },
        },
        getGameRows() {
            if (!this._gameRows) {
                this._gameRows = document.querySelectorAll(
                    SELECTORS.GAME_BOARD + ' > div'
                );
            }
            return this._gameRows;
        },
        getGuessRows() {
            if (!this._guessRows) {
                this._guessRows = document.querySelectorAll(
                    SELECTORS.GUESS_ROW
                );
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
            this.initToast();
            this.setupHTMXHandlers();
            setTimeout(() => this.updateGameState(), 100);
        },
        initToast() {
            const toastElement = document.querySelector(
                SELECTORS.NOTIFICATION_TOAST
            );
            if (
                toastElement &&
                typeof bootstrap !== 'undefined' &&
                bootstrap.Toast
            ) {
                this._toast = new bootstrap.Toast(toastElement, {
                    delay: 3000,
                });
            }
        },
        resetGameState() {
            this.currentGuess = '';
            this.currentRow = 0;
            this.gameOver = false;
            this.hintVisible = false;
            this.keyStatus = {};
            this.submittingGuess = false;
            this.clearDOMCache();
        },
        initTheme() {
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        },
        _handleTriggerHeader(header) {
            if (!header) {
                this.lastServerError = '';
                this.updateGameState();
                return;
            }

            try {
                const parsed = JSON.parse(header);
                if (typeof parsed['clear-completed-words'] !== 'undefined') {
                    this.clearCompletedWords();
                }
                if (parsed.server_error_code) {
                    const code = parsed.server_error_code;
                    const info = this.errorCodeMessages[code] || {
                        text: `An unexpected error occurred. (code: ${code}) ❗`,
                        type: 'error',
                    };
                    this.lastServerError = code;
                    this.keepInputAfterError = true;
                    this.showToastNotification(info.text, info.type);
                    this.submittingGuess = false;
                    this.shakeCurrentRow();
                } else {
                    this.lastServerError = '';
                    this.updateGameState();
                }
            } catch {
                if (header.includes('clear-completed-words')) {
                    this.clearCompletedWords();
                }
                if (!header.includes('server_error_code')) {
                    this.lastServerError = '';
                    this.updateGameState();
                }
            }
        },
        setupHTMXHandlers() {
            document.body.addEventListener('htmx:afterSwap', (evt) => {
                this.submittingGuess = false;
                this.clearDOMCache();
                this.restoreUserInput();
                const targetEl =
                    evt?.detail?.target ||
                    document.querySelector(SELECTORS.GAME_CONTENT_CONTAINER);
                if (
                    window.Alpine &&
                    targetEl &&
                    typeof window.Alpine.initTree === 'function'
                ) {
                    window.Alpine.initTree(targetEl);
                }
            });

            document.body.addEventListener('htmx:beforeSwap', () => {
                if (this.currentGuess) {
                    this.tempCurrentGuess = this.currentGuess;
                    this.tempCurrentRow = this.currentRow;
                }
            });

            document.body.addEventListener('htmx:responseError', (evt) => {
                const message =
                    evt.detail.xhr.status === 429
                        ? 'Too many requests. Please slow down!'
                        : 'Connection error. Please try again!';
                this.showToastNotification(message, 'warning');
            });

            document.body.addEventListener('htmx:sendError', () => {
                this.showToastNotification(
                    'Network error. Check your connection!',
                    'error'
                );
            });

            document.body.addEventListener('htmx:timeout', () => {
                this.showToastNotification(
                    'Request timed out. Please try again!',
                    'error'
                );
            });

            document.body.addEventListener('htmx:afterSettle', (evt) => {
                const xhr = evt?.detail?.xhr;
                if (xhr && typeof xhr.getResponseHeader === 'function') {
                    this._handleTriggerHeader(
                        xhr.getResponseHeader('HX-Trigger')
                    );
                } else {
                    this.updateGameState();
                }
            });

            if (window.htmx) {
                htmx.on('htmx:configRequest', (evt) => {
                    let token = readCookie('csrf_token');
                    if (!token) {
                        const meta = document.querySelector(
                            SELECTORS.CSRF_META
                        );
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
            }
            this.tempCurrentGuess = null;
            this.tempCurrentRow = null;
        },
        updateDisplay: debounce(function () {
            const row = this.getGameRows()?.[this.currentRow];
            if (!row) return;

            const tiles = row.querySelectorAll(SELECTORS.TILE);
            tiles?.forEach((tile, i) => {
                tile.classList.remove(
                    CSS_CLASSES.CORRECT,
                    CSS_CLASSES.PRESENT,
                    CSS_CLASSES.ABSENT
                );
                const letter = this.currentGuess[i] || '';
                tile.textContent = letter;
                if (letter) {
                    tile.classList.add(CSS_CLASSES.FILLED);
                } else {
                    tile.classList.remove(CSS_CLASSES.FILLED);
                }
            });
        }, 50),
        shakeCurrentRow() {
            const row = this.getGuessRows()?.[Math.max(0, this.currentRow)];
            if (row) {
                row.classList.add(CSS_CLASSES.SHAKE);
                setTimeout(() => row.classList.remove(CSS_CLASSES.SHAKE), 500);
            }
        },
        handleKeyInput(key, evt) {
            if (this.gameOver) {
                this.showToastNotification(
                    'Game is over! Start a new game to continue!',
                    'warning'
                );
                return;
            }
            if (evt?.target instanceof HTMLButtonElement) {
                evt.target.disabled = true;
                evt.target.classList.add(CSS_CLASSES.PRESSED);
                setTimeout(() => {
                    evt.target.disabled = false;
                    evt.target.classList.remove(CSS_CLASSES.PRESSED);
                }, 120);
            }
            if (key === 'Enter' || key === 'ENTER') {
                this.submitGuess();
            } else if (key === 'Backspace' || key === 'BACKSPACE') {
                this.deleteLetter();
            } else if (REGEX.LETTER.test(key)) {
                this.addLetter(key.toUpperCase());
            }
        },
        handleKeyPress(e) {
            this.handleKeyInput(e.key);
        },
        handleVirtualKey(key, evt) {
            this.handleKeyInput(key, evt);
        },
        addLetter(letter) {
            if (this.currentGuess.length < WORD_LENGTH) {
                this.currentGuess += letter;
                this.updateDisplay();
            } else {
                this.showToastNotification(
                    `Word is already ${WORD_LENGTH} letters! Press Enter to submit!`,
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
            const board = document.querySelector(SELECTORS.GAME_BOARD);
            if (!board) return;

            const errEl = board.querySelector('[data-error-code]');
            if (errEl) {
                const code = errEl.getAttribute('data-error-code');
                if (code) {
                    const info = this.errorCodeMessages[code] || {
                        text: `An unexpected error occurred. (code: ${code})`,
                        type: 'error',
                    };
                    this.showToastNotification(info.text, info.type);
                    this.shakeCurrentRow();
                }
            }

            if (this.keepInputAfterError) {
                this.keepInputAfterError = false;
            } else {
                this.currentGuess = '';
            }

            const gameOverContainer = board.parentElement.querySelector(
                '.mt-3.p-3.bg-body-secondary'
            );
            const wasGameOver = this.gameOver;
            this.gameOver = gameOverContainer !== null;
            if (!wasGameOver && this.gameOver) {
                this.hintVisible = false;
            }

            const rows = this.getGuessRows();
            let completedRows = 0;
            rows.forEach((row) => {
                const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
                const hasStatusTiles = Array.from(tiles).some(
                    (tile) =>
                        tile.classList.contains(CSS_CLASSES.CORRECT) ||
                        tile.classList.contains(CSS_CLASSES.PRESENT) ||
                        tile.classList.contains(CSS_CLASSES.ABSENT)
                );
                if (hasStatusTiles) {
                    completedRows++;
                }
            });

            this.currentRow = Math.min(completedRows, rows.length - 1);
            this.updateKeyboardColors(rows);
            this.animateNewGuess(rows);
            this.checkForWin(rows, gameOverContainer);
        },
        submitGuess() {
            if (
                this.submittingGuess ||
                this.gameOver ||
                this.currentGuess.length !== WORD_LENGTH
            ) {
                if (this.submittingGuess) {
                    this.showToastNotification(
                        'Please wait, submitting your guess...',
                        'info'
                    );
                } else if (this.currentGuess.length < WORD_LENGTH) {
                    this.showToastNotification(
                        `Word must be ${WORD_LENGTH} letters long!`,
                        'warning'
                    );
                } else if (this.gameOver) {
                    this.showToastNotification(
                        'Game is already over! Start a new game!',
                        'warning'
                    );
                }
                this.shakeCurrentRow();
                return;
            }
            this.submittingGuess = true;
            const guessInput = document.querySelector(SELECTORS.GUESS_INPUT);
            if (guessInput) {
                guessInput.value = this.currentGuess;
            }
            htmx.trigger(SELECTORS.GUESS_FORM, 'submit');
        },
        updateKeyboardColors(rows) {
            this.keyStatus = {};
            rows.forEach((row) => {
                const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
                tiles.forEach((tile) => {
                    const letter = tile.textContent;
                    const status = tile.classList.contains(CSS_CLASSES.CORRECT)
                        ? 'correct'
                        : tile.classList.contains(CSS_CLASSES.PRESENT)
                        ? 'present'
                        : tile.classList.contains(CSS_CLASSES.ABSENT)
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
            });
        },
        getKeyClass(letter) {
            return this.keyStatus[letter] ?? '';
        },
        animateNewGuess(allRows) {
            const rows = allRows || this.getGameRows();
            const row = rows?.[this.currentRow - 1];
            if (!row || row.classList.contains(CSS_CLASSES.ANIMATED)) return;

            const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
            if (tiles.length !== WORD_LENGTH) return;

            tiles.forEach((tile, index) => {
                tile.style.setProperty('--tile-index', index);
                setTimeout(() => {
                    tile.classList.add(CSS_CLASSES.FLIP);
                    setTimeout(
                        () => tile.classList.add(CSS_CLASSES.FLIP_REVEALED),
                        300
                    );
                }, index * ANIMATION_DELAY);
            });
            row.classList.add(CSS_CLASSES.ANIMATED);
            row.classList.remove('submitting');

            setTimeout(() => {
                const sr = document.querySelector(SELECTORS.SR_LIVE);
                if (sr) {
                    const parts = Array.from(tiles).map((tile) => {
                        const letter = tile.textContent || '';
                        const status = tile.classList.contains(
                            CSS_CLASSES.CORRECT
                        )
                            ? 'correct'
                            : tile.classList.contains(CSS_CLASSES.PRESENT)
                            ? 'present'
                            : tile.classList.contains(CSS_CLASSES.ABSENT)
                            ? 'absent'
                            : 'unknown';
                        return `${letter} is ${status}`;
                    });
                    sr.textContent = `Row ${
                        this.currentRow
                    } revealed: ${parts.join(', ')}.`;
                }
            }, WORD_LENGTH * ANIMATION_DELAY + 400);
        },
        checkForWin(allRows, gameOverContainer) {
            const rows = allRows || this.getGameRows();
            const winningRow = Array.from(rows).find(
                (row) =>
                    row.querySelectorAll('.' + CSS_CLASSES.CORRECT).length ===
                    WORD_LENGTH
            );

            if (winningRow) {
                this.gameOver = true;
                if (!winningRow.classList.contains(CSS_CLASSES.WINNER)) {
                    winningRow.classList.add(CSS_CLASSES.WINNER);
                    winningRow
                        .querySelectorAll(SELECTORS.TILE)
                        .forEach((tile, index) => {
                            tile.style.setProperty('--tile-index', index);
                        });
                }
                this.launchConfetti();
                const word = Array.from(
                    winningRow.querySelectorAll('.' + CSS_CLASSES.CORRECT)
                )
                    .map((tile) => tile.textContent)
                    .join('');
                if (word) {
                    this.saveCompletedWord(word.toUpperCase());
                }
            } else {
                const completedRowCount = Array.from(rows).filter((row) => {
                    const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
                    return (
                        tiles.length === WORD_LENGTH &&
                        Array.from(tiles).some(
                            (tile) =>
                                tile.classList.contains(CSS_CLASSES.CORRECT) ||
                                tile.classList.contains(CSS_CLASSES.PRESENT) ||
                                tile.classList.contains(CSS_CLASSES.ABSENT)
                        )
                    );
                }).length;

                if (completedRowCount === MAX_GUESSES) {
                    setTimeout(
                        () =>
                            this.showToastNotification(
                                'Game over! Better luck next time!',
                                'info'
                            ),
                        1000
                    );
                    const solution = gameOverContainer?.dataset.solution;
                    if (solution) {
                        this.saveCompletedWord(solution.toUpperCase());
                    }
                }
            }
        },
        launchConfetti() {
            if (
                !window._confettiScriptLoaded &&
                typeof window.confetti !== 'function'
            ) {
                window._confettiScriptLoaded = true;
                const script = document.createElement('script');
                script.src =
                    'https://cdn.jsdelivr.net/npm/canvas-confetti@1.9.3/dist/confetti.browser.min.js';
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
                            CONFETTI_COLORS[
                                Math.floor(
                                    Math.random() * CONFETTI_COLORS.length
                                )
                            ],
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
            let completedRowCount = 0;
            let hasWon = false;

            rows.forEach((row) => {
                const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
                if (tiles.length === WORD_LENGTH) {
                    completedRowCount++;
                    if (
                        row.querySelectorAll('.' + CSS_CLASSES.CORRECT)
                            .length === WORD_LENGTH
                    ) {
                        hasWon = true;
                    }
                }
            });

            let emojiGrid = `Vortludo ${
                hasWon ? completedRowCount : 'X'
            }/6\n\n`;

            rows.forEach((row) => {
                const tiles = row.querySelectorAll(SELECTORS.FILLED_TILE);
                if (tiles.length === WORD_LENGTH) {
                    tiles.forEach((tile) => {
                        if (tile.classList.contains(CSS_CLASSES.CORRECT)) {
                            emojiGrid += '🟩';
                        } else if (
                            tile.classList.contains(CSS_CLASSES.PRESENT)
                        ) {
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
        async copyToClipboard(text) {
            try {
                if (navigator.clipboard && window.isSecureContext) {
                    await navigator.clipboard.writeText(text);
                    this.showToastNotification(
                        'Results copied to clipboard!',
                        'success'
                    );
                    return;
                }
                this.openCopyModal(text);
            } catch {
                this.openCopyModal(text);
                this.showToastNotification(
                    'Could not copy to clipboard automatically.',
                    'warning'
                );
            }
        },
        openCopyModal(text) {
            this.copyModalText = text;
            const modalEl = document.querySelector(SELECTORS.COPY_MODAL);
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
            if (this._bsModal) {
                this._bsModal.hide();
            } else {
                this.showCopyModal = false;
            }
            this.copyModalText = '';
        },
        selectAllText() {
            const textarea = document.querySelector(
                SELECTORS.COPY_MODAL_TEXTAREA
            );
            if (textarea) {
                textarea.select();
                textarea.focus();
            } else {
                this.showToastNotification(
                    'Could not select text for copying.',
                    'warning'
                );
            }
        },
        showToastNotification(message, type = 'info') {
            this.toastMessage = message;
            this.toastType = type;
            this.$nextTick(() => {
                if (this._toast) {
                    this._toast.show();
                }
            });
        },
        shouldHideKeyboard() {
            return this.gameOver;
        },
        getCompletedWords() {
            try {
                const completed = localStorage.getItem(COMPLETED_WORDS_KEY);
                return completed ? JSON.parse(completed) : [];
            } catch {
                this._storageErrorToast('load');
                return [];
            }
        },
        saveCompletedWord(word) {
            try {
                const completed = this.getCompletedWords();
                if (!completed.includes(word)) {
                    completed.push(word);
                    localStorage.setItem(
                        COMPLETED_WORDS_KEY,
                        JSON.stringify(completed)
                    );
                    this.showToastNotification(
                        `Word "${word}" added to your completed list!`,
                        'success'
                    );
                }
            } catch {
                this._storageErrorToast('save');
            }
        },
        clearCompletedWords() {
            try {
                localStorage.removeItem(COMPLETED_WORDS_KEY);
                this.showToastNotification(
                    "🎉 Congratulations! You've completed all words! Progress reset.",
                    'success'
                );
            } catch {
                this._storageErrorToast('clear');
            }
        },
        _storageErrorToast(action) {
            const messages = {
                load: 'Could not load completed words from your browser storage.',
                save: 'Could not save completed word to your browser storage.',
                clear: 'Could not clear completed words from your browser storage.',
            };
            this.showToastNotification(
                messages[action] || 'Storage error.',
                'warning'
            );
        },
        prepareNewGameData(event) {
            const completedWords = this.getCompletedWords();
            const form = event.target;
            const completedWordsInput = form.querySelector(
                'input[name="completedWords"]'
            );
            if (completedWordsInput && completedWords.length > 0) {
                completedWordsInput.value = JSON.stringify(completedWords);
            }
        },
    };
};

window.shareResults = function () {
    if (window.Alpine && typeof window.Alpine.$data === 'function') {
        const xDataEl = document.querySelector('[x-data]');
        if (xDataEl) {
            const alpineData = window.Alpine.$data(xDataEl);
            if (alpineData && typeof alpineData.shareResults === 'function') {
                alpineData.shareResults();
            }
        }
    }
};
