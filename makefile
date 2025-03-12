SRC := $(wildcard cmd/*) $(wildcard pkg/*/*.go)
BIN := grogbarrel

.PHONY: all
all: $(BIN)

$(BIN): $(SRC)
	go build -o $@ ./cmd/grog_barrel.go

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm $(BIN)
	go mod tidy

.PHONY: info
info:
	@echo SRC: $(SRC)
	@echo BIN: $(BIN)

.PHONY: help
help:
	@echo "TARGETS:"
	@echo "\tgrogbarrel\t\t\t"
	@echo "\ttest\t\t\trun all tests"
	@echo "\tclean"
	@echo "\tinfo"
	@echo "\thelp\t\t\tprint this help"
