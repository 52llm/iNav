.PHONY: web build test

web:
	cd web && npm install && npm run build

build: web
	go build -o inav .

test:
	go test ./...
	cd web && npm run test
