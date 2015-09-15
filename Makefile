GODEP=$(GOPATH)/bin/godep
# Hidden directory to install dependencies for jenkins
export PATH := ./bin:$(PATH)

.PHONY: build
build:
	go get github.com/tools/godep
	$(GODEP) go build ./...

.PHONY: deps
deps:
	go get -t ./...
	# goautotest is used from the Makefile to run tests in a loop
	go get github.com/tsg/goautotest
	# cover
	go get golang.org/x/tools/cmd/cover

.PHONY: gofmt
gofmt:
	go fmt ./...

.PHONY: test
test:
	$(GODEP) go test -short ./...

.PHONY: autotest
autotest:
	goautotest -short ./...

.PHONY: testlong
testlong:
	go vet ./...
	make coverage

.PHONY: benchmark
benchmark:
	go test -short -bench=. ./...

.PHONY: coverage
coverage:
	# gotestcover is needed to fetch coverage for multiple packages
	go get github.com/pierrre/gotestcover
	GOPATH=$(shell $(GODEP) path):$(GOPATH) $(GOPATH)/bin/gotestcover -coverprofile=profile.cov -covermode=count github.com/elastic/libbeat/...
	mkdir -p coverage
	$(GODEP) go tool cover -html=profile.cov -o coverage/coverage.html

.PHONY: clean
clean:
	make gofmt
	-rm profile.cov
	-rm -r coverage


# Builds the environment to test libbeat
.PHONY: build-image
build-image:
	make clean
	docker-compose build

# Runs the environment so the redis and elasticsearch can also be used for local development
# To use it for running the test, set ES_HOST and REDIS_HOST environment variable to the ip of your docker-machine.
.PHONY: start-environment
start-environment: stop-environment
	docker-compose up -d redis elasticsearch
	
.PHONY: stop-environment
stop-environment:
	-docker-compose stop
	-docker-compose rm -f
	-docker ps -a  | grep libbeat | grep Exited | awk '{print $$1}' | xargs docker rm

# Runs the full test suite and puts out the result. This can be run on any docker-machine (local, remote)
.PHONY: testsuite
testsuite: build-image
	docker-compose run libbeat make testlong
	# Copy coverage file back to host
	docker cp libbeat_libbeat_run_1:/go/src/github.com/elastic/libbeat/profile.cov $(shell pwd)/
	mkdir -p coverage
	docker cp libbeat_libbeat_run_1:/go/src/github.com/elastic/libbeat/coverage/coverage.html $(shell pwd)/coverage/

# Sets up docker-compose locally for jenkins so no global installation is needed
.PHONY: testsuite
docker-compose-setup:
	mkdir -p bin
	curl -L https://github.com/docker/compose/releases/download/1.4.0/docker-compose-`uname -s`-`uname -m` > bin/docker-compose
	chmod +x bin/docker-compose
	
