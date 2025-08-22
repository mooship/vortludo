# Copilot Instructions

## Project Overview

This is a web application built with Go backend using Gin framework, and frontend using Alpine.js, HTMX, and CSS. The project follows modern web development patterns with server-side rendering and progressive enhancement.

## Technology Stack

- **Backend**: Go with Gin framework
- **Frontend**: Alpine.js, HTMX, CSS
- **Architecture**: Server-side rendering with HTMX for dynamic interactions

## Code Style and Conventions

### Go/Gin Backend

- Use standard Go formatting with go fmt
- Follow effective Go naming conventions (camelCase for private, PascalCase for public)
- Structure handlers in logical groups
- Use Gin middleware for cross-cutting concerns like logging, CORS, and authentication
- Prefer dependency injection for handlers
- Use context.Context for request scoping
- Keep handlers thin and delegate business logic to service layers
- Return HTML fragments for HTMX requests and JSON for Alpine.js AJAX requests

### HTML Templates

- Use Go's html/template for server-side rendering
- Structure templates with partials and layouts for maintainability
- Include HTMX attributes directly in HTML elements
- Use Alpine.js directives for client-side interactivity
- Follow semantic HTML practices
- Ensure templates work without JavaScript for progressive enhancement

### HTMX Patterns

- Use hx-get, hx-post, hx-put, hx-delete for HTTP requests
- Leverage hx-target and hx-swap for precise DOM updates
- Use hx-trigger for custom event handling patterns
- Implement hx-indicator for loading states and user feedback
- Design endpoints to return HTML fragments specifically for HTMX consumption
- Use hx-boost for progressive enhancement of regular links and forms

### Alpine.js Patterns

- Use x-data for component-level state management
- Implement x-show and x-if for conditional rendering
- Use event handlers like @click and @input for user interactions
- Leverage x-model for two-way data binding
- Keep Alpine components focused and lightweight
- Use Alpine for UI state that doesn't require server interaction
- Implement proper cleanup and memory management for Alpine components

### CSS Styling

- Use CSS custom properties for theming and design consistency
- Implement responsive design with mobile-first approach
- Use CSS Grid and Flexbox for modern layouts
- Keep styles modular and component-focused
- Follow BEM or similar naming conventions for maintainability
- Optimize for performance with efficient selectors
- Use modern CSS features like container queries where appropriate

## File Structure

Organize the project with clear separation of concerns:
- Main Go application file in project root
- Handlers grouped by feature in dedicated directory
- Services layer for business logic
- Models for data structures
- Templates organized with layouts, partials, and pages
- Static assets separated by type (CSS, JS, images)
- Public directory for compiled/optimized assets

## API Design Patterns

- Use RESTful routes where appropriate and logical
- Return HTML fragments for HTMX requests
- Return JSON for Alpine.js AJAX requests
- Include proper HTTP status codes in all responses
- Use middleware for authentication, validation, and logging
- Design endpoints with both traditional and HTMX usage in mind
- Implement proper error handling with user-friendly messages

## Progressive Enhancement Strategy

- Start with working HTML forms and server-side functionality
- Enhance forms and interactions with HTMX for seamless user experience
- Add Alpine.js for rich client-side behavior and state management
- Ensure core functionality works without JavaScript enabled
- Layer enhancements progressively from basic to advanced

## Performance Considerations

- Use HTMX for partial page updates to reduce bandwidth
- Implement proper HTTP caching headers for static assets
- Optimize images and static assets for web delivery
- Use Alpine.js judiciously to avoid performance bottlenecks
- Consider lazy loading patterns for large datasets
- Minimize JavaScript bundle size and execution time

## Error Handling Approach

- Return appropriate HTML error fragments for HTMX requests
- Use Alpine.js for client-side error display and user feedback
- Implement comprehensive logging in Go handlers
- Show user-friendly error messages while logging technical details
- Handle network errors gracefully in both HTMX and Alpine contexts

## Security Best Practices

- Use CSRF protection with Gin middleware
- Validate and sanitize all inputs server-side
- Escape HTML output properly to prevent XSS
- Use HTTPS in production environments
- Implement proper authentication and authorization
- Follow OWASP guidelines for web application security

## Development Workflow

- Use Go's built-in tooling for development and testing
- Leverage browser developer tools for debugging HTMX requests
- Use Alpine.js devtools browser extension for debugging
- Test functionality with JavaScript disabled
- Implement automated testing for backend logic
- Use hot reloading for efficient development

## Integration Patterns

- Combine HTMX and Alpine.js thoughtfully, using each for their strengths
- Use HTMX for server communication and page updates
- Use Alpine.js for complex client-side UI state and interactions
- Ensure smooth handoff between server-rendered and client-enhanced content
- Design components that work well with both technologies

## Code Generation Preferences

- Always provide complete, production-ready code without examples or samples
- Generate the final implementation directly, not demonstrations or tutorials
- Skip explanatory examples and provide only the working solution
- Deliver finished code that can be used immediately in the project
- Avoid showing "how to" examples - just implement the requested functionality
- Provide the actual code needed, not instructional or educational samples
- Focus on delivering final implementations rather than teaching patterns