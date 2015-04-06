.PHONY: build
build: 
	go build ./...

.PHONY: deps
deps:
	go get -t ./...
	# goautotest is used from the Makefile to run tests in a loop
	go get github.com/tsg/goautotest

.PHONY: gofmt
gofmt:
	go fmt ./...

.PHONY: test
test:
	go test -short ./...

.PHONY: autotest
autotest:
	goautotest -short ./...

.PHONY: testlong
testlong:
	go vet ./...
	go test ./...

.PHONY: benchmark
benchmark:
	go test -short -bench=. ./...
