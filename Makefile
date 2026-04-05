SHELL := /bin/bash

.PHONY: install-local deploy-from-env diagnose start stop healthcheck build

install-local:
	bash deploy/scripts/bootstrap.sh

deploy-from-env:
	bash deploy/scripts/bootstrap.sh --from-env

diagnose:
	bash deploy/scripts/doctor.sh

start:
	bash deploy/scripts/start.sh

stop:
	bash deploy/scripts/stop.sh

healthcheck:
	bash deploy/scripts/healthcheck.sh

build:
	PATH="/opt/homebrew/opt/go@1.24/bin:$$PATH" go build ./...
