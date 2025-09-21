#!/bin/bash

set -euf

OWNER='banzaicloud'
REPO='banzaicloud.github.io'
WORKFLOW='submodule-update.yml'

function main()
{
    curl \
      -X POST \
      -H "Accept: application/vnd.github+json" \
      -H "Authorization: token ${GITHUB_TOKEN}" \
      "https://api.github.com/repos/${OWNER}/${REPO}/actions/workflows/${WORKFLOW}/dispatches" \
      -d '{"ref":"gh-pages","inputs":{"module":"kafka-operator"}}'
}

main "$@"
