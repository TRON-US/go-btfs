* **What kind of change does this PR introduce?** (Bug fix, feature, docs update, ...)

  Docs update.

* **What is the current behavior?** (You can also link to an open issue here)

  There is only the pull request template, but no clear example of how to actually use iit.

* **What is the new behavior?** (You can also refer to a JIRA ticket here)

  The pull request template has an example under .github/example_pull_request.md.

  BTFS-734 is also happy about this change.

* **Does this PR introduce a breaking change?** (What changes might users need to make in their application due to this PR?)

  No. Users can continue using their BTFS nodes without updating anything.

* **What dependencies / modules need to be updated as pre-requisites?** (Include relevant PR links here)

  N/A.

* **Description of changes**

  - Added new example_pull_request.md to illustrate the usage of a pull request template
  - Filled in all the blanks to show case the scope of answers
  - BTFS is consistently awesome

---

* **Please check if the PR fulfills these requirements**
- [x] The subject of this PR contains a JIRA ticket BTFS-xxxx (N/A for community PRs)
- [x] PR description and commit messages are [good and meaningful](https://chris.beams.io/posts/git-commit/)
- [x] Unit tests for the changes have been added (for bug fixes / features)
- [x] Docs have been added / updated (for bug fixes / features)

* **Please make sure the following procedures have been applied before requesting reviewers**
- [x] Code changes closely follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [x] Code has been *rebased* against latest master (`git merge` not recommended, unless you know what you are doing)
- [x] Code changes have run through `go fmt`
- [x] Code changes have run through `go mod tidy`
- [x] All unit tests passed locally (`make test_go_test`)
