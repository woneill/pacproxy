NAME = $(shell awk -F\" '/^const Name/ { print $$2 }' ./pacproxy.go)
VERSION = $(shell awk -F\" '/^const Version/ { print $$2 }' ./pacproxy.go)

all: build

build:
	@mkdir -p bin/
	go build -race -o bin/$(NAME)

test:
	go test -race

xcompile: deps test
	@rm -rf build/
	@mkdir -p build
	gox -output="build/{{.Dir}}_$(VERSION)_{{.OS}}_{{.Arch}}/$(NAME)"

package: xcompile
	$(eval FILES := $(shell ls build))
	@mkdir -p build/tgz
	for f in $(FILES); do \
		(cd $(shell pwd)/build && tar -zcvf tgz/$$f.tar.gz $$f); \
		echo $$f; \
	done

clean:
	@rm -rf bin/
	@rm -rf build/

.PHONY: all build test xcompile package clean
