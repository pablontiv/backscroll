---
estado: Completed
tipo: task
---
# T004: Configure branch protection on main

**Contribuye a**: enforce the ecosystem security baseline — all repos must require PR review before any commit reaches the default branch.

## Alcance

**In**:
- Configure branch protection on `main` via GitHub API:
  - `required_pull_request_reviews.required_approving_review_count: 1`
  - `required_pull_request_reviews.dismiss_stale_reviews: true`
  - `allow_force_pushes: false`
  - `allow_deletions: false`
- Enable `required_status_checks` requiring CI and gitleaks to pass before merge

**Out**:
- No file changes in the repo

## Criterios de Aceptación

- `gh api repos/pablontiv/backscroll/branches/main/protection` returns HTTP 200 (not 404)
- `required_pull_request_reviews.required_approving_review_count` >= 1
- `allow_force_pushes.enabled` = false
- `allow_deletions.enabled` = false

## Fuente de verdad

- GitHub API: PUT repos/pablontiv/backscroll/branches/main/protection
