FROM golang:alpine AS builder

# Install git
RUN apk update && apk add --no-cache git

WORKDIR $GOPATH/src/cli-experimental
COPY . .

# Install wire
RUN GO111MODULE=off go get github.com/google/wire/cmd/wire

ENV CGO_ENABLED=0

# Run go generate
RUN GO111MODULE=on go generate

# Build binary
RUN GO111MODULE=on  go build -o /go/bin/k2

FROM scratch

# Copy the executable.
COPY --from=builder /go/bin/k2 /go/bin/k2

# Run the binary
ENTRYPOINT ["go/bin/k2"]


