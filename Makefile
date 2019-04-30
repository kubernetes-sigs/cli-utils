generate:
	GO111MODULE=on go generate

run:
	GO111MODULE=on go run .

build:
	GO111MODULE=on go build .

check:
	GO111MODULE=on ./scripts/check-everything.sh
