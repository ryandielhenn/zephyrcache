# Contributing to ZephyrCache

Thank you for your interest in contributing to ZephyrCache. We appreciate your help in making this project better.

---

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct. We are committed to providing a welcoming and inclusive experience for everyone. Please treat all contributors with respect and kindness.

## How Can I Contribute?

We welcome contributions in many forms:

- **Bug Reports:** Let us know if something isn't working right.
- **Feature Requests:** Propose new ideas or enhancements.
- **Documentation:** Help us improve our guides, tutorials, and READMEs.
- **Code:** Fix issues or implement new features via Pull Requests.

---

## Development Workflow

### 1. Fork and Clone

Fork the repository to your own GitHub account, then clone it to your local machine:

```bash
git clone https://github.com/<your-username>/zephyrcache.git
cd zephyrcache
```

### 2. Syncing Your Fork with Upstream

Add a git remote for syncing your fork with the upstream repository:

```bash
git remote add upstream git@github.com:ryandielhenn/zephyrcache.git
```

> **Tip:** You can add this alias to your shell config (e.g., `~/.bashrc` or `~/.zshrc`) to make syncing easier. Feel free to rename it to whatever works for you!
>
> ```bash
> alias update="git checkout main \
>   && git fetch --tags -f upstream \
>   && git rebase upstream/main \
>   && git push origin HEAD:main"
> ```

### 3. Create a Branch

Create a branch for your work using a descriptive name:

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b bugfix/issue-number
```

### 4. Coding Standards

To keep the codebase clean, please follow these guidelines:

- **Style and Formatting:** Run the formatter and linter before committing:

  ```bash
  # Format all Go files in the project
  go fmt ./...

  # Run the linter (install once if needed: https://golangci-lint.run/welcome/install)
  golangci-lint run ./...
  ```

  Fix any warnings or errors before pushing. If a linter rule seems like a false positive, note it in your PR description.

- **Naming Conventions:** Use clear, descriptive names for variables, functions, and types.
- **Commit Messages:** Keep messages concise and descriptive (e.g., `fix: resolve cache eviction race condition`).

### 5. Testing Requirements

We require all code changes to be verified:

- **Run Existing Tests:** Ensure your changes do not break existing functionality.

  ```bash
  go test ./...
  ```

- **Add New Tests:** Include unit tests for any new logic or bug fixes.
- **Verification:** Ensure all tests pass locally before pushing your changes.

### 6. Submitting a Pull Request

1. Push your changes to your fork.
2. Open a Pull Request (PR) against the `main` branch.
3. In the PR description, clearly explain what was changed and why.
4. Reference any related issues (e.g., `Closes #123`).

---

## Issue Reporting

When creating a new issue, please include the following information:

1. **Project Version:** Specify which version of the project you are using.
2. **Reproduction Steps:** Provide a clear list of steps to trigger the bug.
3. **Expected vs. Actual Behavior:** Describe what you expected to happen and what actually occurred.
4. **Environment:** Include details about your operating system and relevant hardware/software.

---

## Getting Help

If you have questions or need guidance:

- **Issue Comments:** Comment on the relevant issue and tag a maintainer for clarification.

Thank you for being part of our community.
