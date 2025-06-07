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

window.gameApp = function () {
    return {
        WORD_LENGTH: 5,
        MAX_GUESSES: 6,
        ANIMATION_DELAY: 100,
        TOAST_DURATION: 3000,
        currentGuess: '',
        currentRow: 0,
        gameOver: false,
        isDarkMode: false,
        keyStatus: {},
        showCopyModal: false,
        copyModalText: '',
        showToast: false,
        toastMessage: '',
        toastType: 'primary',
        submittingGuess: false,
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
        },
        initTheme() {
            const savedTheme = localStorage.getItem('theme') || 'light';
            this.isDarkMode = savedTheme === 'dark';
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        },
        setupHTMXHandlers() {
            document.body.addEventListener('htmx:afterSwap', (evt) => {
                this.submittingGuess = false;
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
        updateDisplay() {
            const row =
                document.querySelectorAll('#game-board > div')[this.currentRow];
            if (!row) return;
            row.querySelectorAll('.tile').forEach((tile, i) => {
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
        },
        shakeCurrentRow(isInvalid = true) {
            if (!isInvalid) return;
            const rows = document.querySelectorAll('.guess-row');
            const targetRow = Math.max(0, this.currentRow);
            if (rows[targetRow]) {
                rows[targetRow].classList.add('shake');
                setTimeout(
                    () => rows[targetRow].classList.remove('shake'),
                    500
                );
            }
        },
        handleKeyPress(e) {
            if (this.gameOver) return;
            if (e.key === 'Enter') this.submitGuess();
            else if (e.key === 'Backspace') this.deleteLetter();
            else if (/^[a-zA-Z]$/.test(e.key))
                this.addLetter(e.key.toUpperCase());
        },
        handleVirtualKey(key) {
            if (this.gameOver) return;
            const active = event?.target;
            if (active && active.disabled !== undefined) {
                active.disabled = true;
                setTimeout(() => {
                    active.disabled = false;
                }, 120);
            }
            if (key === 'ENTER') this.submitGuess();
            else if (key === 'BACKSPACE') this.deleteLetter();
            else this.addLetter(key);
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
        toggleTheme() {
            this.isDarkMode = !this.isDarkMode;
            const theme = this.isDarkMode ? 'dark' : 'light';
            document.documentElement.setAttribute('data-bs-theme', theme);
            localStorage.setItem('theme', theme);
        },
        canShowHint() {
            const rows = document.querySelectorAll('#game-board > div');
            let completedRows = 0;
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                const hasStatusTiles = Array.from(tiles).some(
                    (tile) =>
                        tile.classList.contains('tile-correct') ||
                        tile.classList.contains('tile-present') ||
                        tile.classList.contains('tile-absent')
                );
                if (hasStatusTiles) completedRows++;
            });
            return completedRows >= 3;
        },
        updateGameState() {
            const board = document.getElementById('game-board');
            if (!board) return;
            this.currentGuess = '';
            this.keyStatus = {};
            const gameOverContainer = board.parentElement.querySelector(
                '.mt-3.p-3.bg-body-secondary'
            );
            this.gameOver = gameOverContainer !== null;
            const rows = document.querySelectorAll('.guess-row');
            let completedRows = 0;
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                const hasStatusTiles = Array.from(tiles).some(
                    (tile) =>
                        tile.classList.contains('tile-correct') ||
                        tile.classList.contains('tile-present') ||
                        tile.classList.contains('tile-absent')
                );
                if (hasStatusTiles) completedRows++;
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
                this.currentGuess.length !== this.WORD_LENGTH
            ) {
                this.shakeCurrentRow(true);
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
            return this.keyStatus[letter] || '';
        },
        animateNewGuess() {
            const rows = document.querySelectorAll('#game-board > div');
            const row = rows[this.currentRow - 1];
            if (!row || row.classList.contains('animated')) return;
            const tiles = row.querySelectorAll('.tile.filled');
            if (tiles.length !== this.WORD_LENGTH) return;
            tiles.forEach((tile, index) => {
                tile.style.setProperty('--tile-index', index);
                setTimeout(() => {
                    tile.classList.add('flip');
                    setTimeout(() => {
                        tile.classList.add('flip-revealed');
                    }, 300);
                }, index * this.ANIMATION_DELAY);
            });
            row.classList.add('animated');
            row.classList.remove('submitting');
        },
        checkForWin() {
            const rows = document.querySelectorAll('#game-board > div');
            let hasWinner = false;
            rows.forEach((row) => {
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
            if (hasWinner) {
                this.gameOver = true;
                this.launchConfetti();
            }
        },
        launchConfetti() {
            if (typeof window.confetti !== 'function') {
                const script = document.createElement('script');
                script.src =
                    'https://cdn.jsdelivr.net/npm/canvas-confetti@1.9.3/dist/confetti.browser.min.js';
                script.onload = () => {
                    this._doConfetti();
                    this._doFireworks();
                };
                document.body.appendChild(script);
            } else {
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
            const rows = document.querySelectorAll('#game-board > div');
            let emojiGrid = 'Vortludo\n\n';
            rows.forEach((row) => {
                const tiles = row.querySelectorAll('.tile.filled');
                if (tiles.length === 5) {
                    tiles.forEach((tile) => {
                        if (tile.classList.contains('tile-correct'))
                            emojiGrid += '🟩';
                        else if (tile.classList.contains('tile-present'))
                            emojiGrid += '🟨';
                        else emojiGrid += '⬛';
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
                    this.showToastNotification('Results copied to clipboard!');
                    return;
                }
                this.openCopyModal(text);
            } catch (err) {
                this.openCopyModal(text);
            }
        },
        openCopyModal(text) {
            this.copyModalText = text;
            this.showCopyModal = true;
        },
        closeCopyModal() {
            this.showCopyModal = false;
            this.copyModalText = '';
        },
        selectAllText() {
            const textarea = document.querySelector('.copy-modal textarea');
            if (textarea) {
                textarea.select();
                textarea.focus();
            }
        },
        showToastNotification(message, isError = false) {
            this.toastMessage = message;
            this.toastType = isError ? 'danger' : 'success';

            this.$nextTick(() => {
                const toastElement =
                    document.getElementById('notification-toast');
                if (toastElement) {
                    if (typeof bootstrap !== 'undefined' && bootstrap.Toast) {
                        const toast = new bootstrap.Toast(toastElement, {
                            delay: 3000,
                        });
                        toast.show();
                    } else {
                        setTimeout(() => {
                            if (
                                typeof bootstrap !== 'undefined' &&
                                bootstrap.Toast
                            ) {
                                const toast = new bootstrap.Toast(
                                    toastElement,
                                    {
                                        delay: 3000,
                                    }
                                );
                                toast.show();
                            }
                        }, 100);
                    }
                }
            });
        },
        shouldHideKeyboard() {
            return this.gameOver;
        },
    };
};

window.shareResults = function () {
    const alpineData = Alpine.$data(document.querySelector('[x-data]'));
    alpineData.shareResults();
};
