# Security and Permission Policy

## Overview

This repository is configured with a strict, defense-in-depth permission architecture ensuring that **only authorized maintainers and the automated GitHub Actions Bot can commit and push directly to `main`**. Any external contributor, fork, or pull request is restricted to read-only access.

---

## 1. Automated Workflow Permissions Architecture

The GitHub Actions workflows implement strict role-based execution barriers:

### A. CI Pipeline (`.github/workflows/ci.yml`)
- **Permission**: `permissions: contents: read`
- **Scope**: Verifies code integrity on `push` and `pull_request` to `main`.
- **Security Boundary**: Pull requests from forks are granted **only read-only tokens** by GitHub Actions. Fork workflows cannot write to repository files or push commits.

### B. Commit Manifest Bot (`.github/workflows/commit_manifest.yml`)
- **Trigger**: Runs **exclusively on `push` to `main`** (and manual `workflow_dispatch`).
- **Forbidden Triggers**: **No `pull_request`** and **No `pull_request_target`** triggers are configured. Pull requests from external forks cannot trigger or run this workflow with write privileges.
- **Actor Gating**:
  ```yaml
  if: >-
    (github.repository_owner == github.actor ||
     github.actor == 'liambrooks-lab' ||
     github.actor == 'R-Jena' ||
     github.actor == 'github-actions[bot]') &&
    !contains(github.event.head_commit.message, '[skip ci]') &&
    !contains(github.event.head_commit.message, 'chore(manifest)')
  ```
- **Token Scope**: `permissions: contents: write` scoped specifically to commit and push the generated `manifests/` updates back to `main`.
- **Loop Prevention**: Employs `[skip ci]` and actor filtering to guarantee no recursive triggers.
- **Concurrency**: `group: commit-manifest`, `cancel-in-progress: false` to ensure atomic, sequential manifest updates.

---

## 2. GitHub Repository Configuration (Recommended Settings)

To complete the defense-in-depth security model on GitHub:

### Branch Protection / Rulesets for `main`
1. Navigate to **Settings** > **Rules** > **Rulesets** (or **Branches** > **Branch protection rules**).
2. Create a rule targeting `main`:
   - **Restrict pushes**: Allow only `@liambrooks-lab` (repo owner) and GitHub Actions.
   - **Require a pull request before merging**:
     - Require at least 1 approval from repository maintainers.
     - Dismiss stale pull request approvals when new commits are pushed.
   - **Require status checks to pass before merging**:
     - Require `Go Lint and Build`, `Python Lint & Compile`, and `Verify Commit Manifest Tooling`.
   - **Do not allow force pushes**: Enabled (checked).
   - **Do not allow deletions**: Enabled (checked).

### Actions Workflow Permissions
1. Navigate to **Settings** > **Actions** > **General**.
2. Under **Workflow permissions**:
   - Select **Read and write permissions** (needed for `commit_manifest.yml` bot commits).
   - Uncheck **Allow GitHub Actions to create and approve pull requests**.
3. Under **Fork pull request workflows**:
   - Select **Require approval for all outside collaborators** or **Require approval for first-time contributors**.

---

## 3. Local Developer Workflow (`Makefile`)

The project Makefile enforces rebasing against `origin/main` before pushing to avoid divergent histories:

- `make push MSG="your commit message"`:
  - Stages all changes (`git add -A`).
  - Commits in a single commit.
  - Fetches `origin/main` and rebases cleanly (`git fetch && git rebase --autostash`).
  - Pushes directly to GitHub in a single commit.
- `make rebase`:
  - Fetches and rebases with `--autostash` to safely handle local unstaged changes.
