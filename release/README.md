# Releasing

Currently (2020/05/06), we are only releasing minor versions of
the cli-utils. We will update these notes, when we begin
updating major/minor/patch versions.

To cut a new cli-utils release perform the following:

- Fetch the latest master changes to a clean branch
  - (Assuming remote fork is named `upstream`)
  - `git checkout -b release`
  - `git fetch upstream`
  - `git reset --hard upstream/master`
- Run `git tag` to determine the latest tag
- Create/Push new tag for release
  - (Assuming updating minor version)
  - `git tag v0.MINOR.0`
  - `git push upstream v0.MINOR.0`
