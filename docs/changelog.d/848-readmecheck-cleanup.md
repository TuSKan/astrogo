---
type: Fixed
pr: 848
---
**`internal/docsguard`'s README check could leave its scratch package behind
on Windows**, where a second test run's repository walkers read it as part of
the module. The cleanup now waits until the directory is gone, and fails the
test and names it if it is not.
