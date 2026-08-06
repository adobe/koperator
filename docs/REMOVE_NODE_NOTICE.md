---
name: Remove Node dependency from gh-pages

This commit removes the Node-related files (package.json & package-lock.json) and
removes "Set up Node" and "Install dependencies" steps from GitHub Actions
workflows so the gh-pages branch no longer depends on Node/npm during CI.
