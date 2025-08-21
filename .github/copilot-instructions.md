# copilot-instructions.md

## 1. Context Usage

* Copilot **must always reference Context7 MCP** for:
  * All decisions
  * Code completions
  * Dependency suggestions
  * UX/UI choices (including frontend and backend)
* Always **verify the latest state of Context7** before acting.

## 2. Formatting & Linting

* For Go:
  * Follow **`gofmt` rules strictly**.
  * Use **idiomatic Go style**.
* For JavaScript/TypeScript:
  * Use **Prettier** for formatting (no ESLint).
  * Use idiomatic, modern JS/TS style.
* Fix linting/formatting issues **preemptively**; no warnings or errors allowed.
* Example (Go):

  ```go
  // Good
  type User struct {
      ID   int
      Name string
  }
  ```

* Example (JS):

  ```js
  // Good
  const user = {
    id: 1,
    name: "Alice",
  };
  ```

## 3. Build & Run Restrictions

* **Never build or run** the app in code generation.
* Copilot only generates or edits **source code** (Go, JS, HTML, CSS, etc).

## 4. Dependencies

* For Go:
  * Check `go.mod` for current dependencies **before suggesting new ones**.
* Always justify new dependencies with **Context7 MCP**.

## 5. Type Safety & File Structure

* For Go:
  * Strictly use **typed structs and interfaces**.
  * Avoid `interface{}` unless absolutely necessary. Prefer concrete types.
  * Use `context.Context` for all network calls or cancellable operations.
  * Use **dedicated `.go` files per domain**.
* For JavaScript:
  * Use clear, well-typed data structures.
  * Organize code by domain (e.g., separate files for UI, logic, helpers).
* For static assets and templates:
  * Place HTML templates in `templates/`, static files in `static/`.

## 6. UI/UX

* All UI/UX choices must reference **Context7 MCP** for best practices.
* Use modern, accessible, and responsive design principles.
* Keep UI code (HTML, CSS, JS) clean, modular, and maintainable.

## 7. Security

* **Never hardcode secrets** or sensitive information in Go, JS, or HTML.
* Use **config/environment variables** for credentials, tokens, and secrets in both backend and frontend.
* Validate all external input strictly (backend and frontend).

## 8. Error Handling

* For Go:
  * Return errors explicitly; never ignore them.
  * Wrap errors with context:

    ```go
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }
    ```
* For JavaScript:
  * Always handle errors (e.g., try/catch, .catch for promises).
  * Provide actionable and clear error messages.
* Validate inputs before processing (Go and JS).

## 9. Confirmation for Large Edits

* Copilot must **prompt the user** before:
  * Major refactors
  * Large code generation
* Always document what will change and why **Context7 MCP** requires it.
