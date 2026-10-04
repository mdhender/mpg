## Repository rules

- Assign every new GitHub issue and PR to `mdhender`.
- Alpha workflow: once a request is complete and all relevant tests pass, commit directly to `main` and push without asking.
  Reference or close the relevant issue in the commit message (e.g. `Closes #30`) when appropriate.
- Bump the patch version in `version.go` in every commit (keep the pre-release label unless told otherwise);
  after pushing, tag the commit `v<version>` (e.g. `v0.7.1-alpha`) and push the tag.

## Randomness

- Never use `math/rand`; use `math/rand/v2`.
  Code that uses randomness must accept a seed and build a local source — never draw from a global source.
