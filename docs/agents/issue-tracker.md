# Issue tracker: GitHub

CommerceOps issues and specifications live in the GitHub Issues tracker for
`Sigmabysoul/commerceops`. Use the `gh` CLI from this repository so it resolves
the configured remote automatically.

## Conventions

- Create an issue with `gh issue create` and a complete title and body.
- Read an issue and its discussion with `gh issue view <number> --comments`.
- List issues with `gh issue list`, requesting labels and comments when a skill
  needs to filter or triage them.
- Comment with `gh issue comment`; change labels with `gh issue edit`.
- Close an issue with a final outcome comment using `gh issue close`.

Use structured command arguments or a body file for multiline issue content.
Never put credentials, private production data, or customer documents in an
issue.

## Pull requests as a triage surface

**PRs as a request surface: no.**

Pull requests are review and delivery artifacts. Skills should not treat them
as incoming feature requests unless this flag is deliberately changed later.

## Skill operations

When a skill says "publish to the issue tracker", create a GitHub issue. When a
skill says "fetch the relevant ticket", read the matching GitHub issue and its
comments.

GitHub issues and pull requests share a number space. If a reference such as
`#42` is ambiguous, check the pull request first and then the issue.

## Wayfinding

For `/wayfinder`, keep the map in one issue labelled `wayfinder:map` and create
child tickets as GitHub sub-issues. Prefer GitHub's native issue dependencies
for blocking relationships. If those features are unavailable, use a task list
in the map and a `Blocked by: #<number>` line in the child issue. Claim a child
by assigning it before starting work, and close it only after recording the
answer and updating the map's decisions.
