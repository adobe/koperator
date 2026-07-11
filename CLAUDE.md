# Project instructions

## License headers

- For brand-new files (no prior Cisco/banzaicloud authorship), use an Adobe-only copyright header:
  ```
  # Copyright 2026 Adobe. All rights reserved.
  ```
  Do not add the "Cisco Systems, Inc. and/or its affiliates" line to new files - that copyright belongs on
  files that originated in the upstream banzaicloud/Cisco codebase, not on code Adobe wrote from scratch.
- `make license-header-check` (addlicense) only validates the Apache-2.0 license body text, not the exact
  copyright owner lines, so an Adobe-only header still passes CI.
- Existing files that already carry the dual Cisco+Adobe header (generated via `make gen-license-header` /
  `hack/boilerplate`) should keep it as-is when merely editing them - only new files get the Adobe-only header.
