SRC := $(wildcard $(wildcard */*.go)) $(wildcard *.go)

.PHONY: all
all: grogbarrel

grogbarrel: $(SRC)
	go build .

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	go mod tidy

.PHONY: info
info:
	@echo SRC: $(SRC)

.PHONY: help
help:
	@echo "TARGETS:"
	@echo "\tgrogbarrel\t\t\t"
	@echo "\ttest\t\t\trun all tests"
	@echo "\tclean"
	@echo "\tinfo"
	@echo "\thelp\t\t\tprint this help"
