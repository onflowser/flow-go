#!/usr/bin/env bash

set -e

export JOB_STARTED=$(date -Iseconds)

case $TEST_CATEGORY in
    unit|crypto-unit|integration-unit|integration)
    ;;
    *)
        echo "Valid test category must be provided."
        exit 1
    ;;
esac

process_results="go run $(realpath ./process_results.go) test-results.json"

cd ..

# checkout specified commit
if [ -n "$COMMIT_SHA" ]
then
    git checkout $COMMIT_SHA
fi

export COMMIT_SHA=$(git rev-parse HEAD)
export COMMIT_DATE=$(git show --no-patch --no-notes --pretty='%cI' $COMMIT_SHA)

make crypto/relic/build

case $TEST_CATEGORY in
    unit)
        make install-mock-generators
        make generate-mocks
        JSON_OUTPUT=true make -s unittest-main | $process_results
    ;;
    crypto-unit)
        cd ./crypto
        JSON_OUTPUT=true make -s test | $process_results
    ;;
    integration-unit)
        cd ./integration
        JSON_OUTPUT=true make -s test | $process_results
    ;;
    integration)
        make docker-build-flow
        cd ./integration
        JSON_OUTPUT=true make -s integration-test | $process_results
    ;;
esac

cat test-results.json

gsutil cp test-results.json gs://$GCS_BUCKET/$COMMIT_SHA-$JOB_STARTED-$TEST_CATEGORY.json