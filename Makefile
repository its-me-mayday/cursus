BINARY := cursus
CMD := ./cmd/cursus

.PHONY: build run test test-coverage coverage-html lint clean

build:
	go build -o $(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./... -race

test-coverage:
	@go list ./... | grep -v 'cmd/' | xargs go test -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{if ($$3 != "100.0%") {print "FAIL: coverage " $$3 " < 100%"; exit 1}}'

coverage-html:
	go tool cover -html=coverage.out -o coverage.html

lint:
	go vet ./...

clean:
	rm -f $(BINARY) coverage.out coverage.html
