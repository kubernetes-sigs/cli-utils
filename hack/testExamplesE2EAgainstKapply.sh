#!/usr/bin/env bash
#
# Copyright 2019 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0

set -o nounset
set -o errexit
set -o pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

succeeded=0
failed=0

function run_test() {
    mdrip -alsologtostderr -v 10 --blockTimeOut 6m0s --mode test \
        --label testE2EAgainstLatestRelease "${1}"
}

for path in examples/alphaTestExamples/*.md; do
    test_name="$(basename "${path}")"
    echo "-----------------------------------"
    echo "Example Test: ${test_name}"
    echo "-----------------------------------"
    if run_test "${path}"; then
        echo
        echo -e "${GREEN}Example Test Succeeded: ${test_name}${NC}"
        let "succeeded+=1"
    else
        echo
        echo -e "${RED}Example Test Failed: ${test_name}${NC}"
        let "failed+=1"
    fi
  echo
done

if [[ ${failed} -gt 0 ]]; then
    echo -e "${RED}Example Tests Complete (succeeded: ${succeeded}, failed: ${failed})${NC}"
    exit 1
else
    echo -e "${GREEN}Example Tests Complete (succeeded: ${succeeded}, failed: ${failed})${NC}"
    exit 0
fi
