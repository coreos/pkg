#!/bin/bash -e

# We use these for testing but don't want to publish them, since we'd need
# to bump the major version
for f in .ci/*; do ln -sf $f .; done

go build ./...
