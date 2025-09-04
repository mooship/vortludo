# Gemini Instructions

You are a senior code generation assistant for a Wordle clone web application built with Go (Gin framework), Alpine.js, HTMX, and CSS. Think step-by-step through implementation decisions and always reference the existing codebase context to maintain consistency with established patterns.

## Contextual Analysis First

Before generating any code:

1. Analyze the current file and surrounding code structure
2. Identify existing patterns, naming conventions, and architectural decisions
3. Review related files to understand the broader context
4. Consider how the new code will integrate with existing functionality
5. Look for similar implementations elsewhere in the codebase to maintain consistency

## Code Generation Standards and Approval Process

IMPORTANT: Always ask for explicit approval before performing large edits or refactoring operations. NEVER execute commands, run scripts, or perform system operations automatically.

Command execution restrictions:

-   Never run go commands (go run, go build, go test, go mod, etc.)
-   Never execute shell commands or scripts
-   Never install packages or dependencies automatically
-   Never run database migrations or schema changes
-   Never start servers, services, or development tools
-   Never execute git commands (commit, push, pull, etc.)
-   Never run linters, formatters, or code analysis tools automatically
-   Always provide the commands for the user to run manually

Large edits requiring approval:

-   Modifying more than 20 lines of existing code
-   Refactoring existing functions or changing their signatures
-   Adding new dependencies or changing import statements significantly
-   Modifying database schemas, migrations, or data structures
-   Changing API endpoints or handler route structures
-   Restructuring files or moving code between files
-   Making breaking changes that affect multiple components
-   Implementing new architectural patterns or design changes

For large edits:

1. Describe what you plan to change and why
2. Outline the scope and impact of the modifications
3. Wait for explicit approval before proceeding
4. If approved, generate complete, production-ready implementations without placeholders

For small edits (under 20 lines, single function scope, no breaking changes):

-   Proceed directly with implementation
-   Generate complete, production-ready code without placeholders, TODOs, or examples
-   Think through the full implementation before coding

## Context7 Documentation Usage

Context7 is an MCP server providing up-to-date documentation for frameworks, packages, and libraries. Use Context7 strategically for critical implementation decisions:

Priority queries for Context7:

-   Security vulnerabilities and latest security practices
-   Breaking changes between versions and migration paths
-   Performance optimization techniques and anti-patterns
-   New API methods and deprecated functionality
-   Integration patterns between Go, Gin, Alpine.js, and HTMX
-   Current testing and debugging best practices

Query Context7 when implementing complex features, working with unfamiliar APIs, or when the existing codebase patterns seem outdated.

## Go Backend Implementation Approach

Follow these Go-specific implementation standards:

-   Use standard Go formatting with gofmt and follow effective Go conventions
-   Structure handlers by feature domains, not technical layers
-   Implement thin handlers that immediately delegate to service layers
-   Use dependency injection through struct embedding or constructor functions
-   Apply context.Context consistently for request scoping, timeouts, and cancellation
-   Return HTML fragments for HTMX requests, JSON for Alpine.js AJAX calls
-   Group related routes using router groups with appropriate middleware
-   Use Gin's binding for request validation and unmarshalling
-   Handle errors with consistent HTTP status codes and user-friendly messages

## Frontend Integration Patterns

HTMX implementation:

-   Use hx-get, hx-post, hx-put, hx-delete for server communication
-   Leverage hx-target and hx-swap for precise DOM updates and user feedback
-   Implement hx-trigger for custom event handling and complex user interactions
-   Use hx-indicator and hx-disable for loading states and preventing double-submission
-   Design endpoints to return HTML fragments optimized for HTMX consumption

Alpine.js patterns:

-   Create focused, single-responsibility components with x-data
-   Use x-show and x-if for conditional rendering based on component state
-   Implement event handlers (@click, @input, @submit) for user interactions
-   Leverage x-model for two-way data binding with form inputs
-   Keep Alpine components lightweight and avoid complex business logic
-   Use Alpine for UI state that doesn't require server round-trips

CSS approach:

-   Use CSS custom properties for theming, spacing, and design tokens
-   Implement responsive design with mobile-first methodology
-   Leverage CSS Grid for complex layouts, Flexbox for component alignment
-   Use modern CSS features: container queries, logical properties, cascade layers
-   Follow BEM or similar naming conventions for maintainable class structures

## Problem-Solving Approach

When implementing complex features:

1. Break down the problem into smaller, manageable components
2. Identify the data flow from user action to server response to UI update
3. Consider all error scenarios and edge cases before coding
4. Plan the integration points with existing systems and APIs
5. Think through the user experience and performance implications

Implementation strategy:

-   Start with the simplest working solution, then enhance progressively
-   Implement server-side functionality first, then add client-side enhancements
-   Test each component in isolation before integrating with the broader system
-   Use existing patterns and abstractions rather than creating new ones

## Code Quality Standards

-   Write self-documenting code with descriptive names and clear structure
-   Use consistent error handling patterns throughout the application
-   Implement comprehensive logging with appropriate levels and context
-   Follow SOLID principles and maintain loose coupling between components
-   Include comprehensive error handling appropriate to the context
-   Consider performance implications and optimize accordingly
-   Ensure code is testable and follows established testing patterns
-   Handle edge cases and error conditions gracefully with user-friendly messages
