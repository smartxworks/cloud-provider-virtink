FROM golang:1.19-alpine AS builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY main.go main.go
COPY pkg/ pkg/
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -a main.go

FROM alpine

COPY --from=builder /workspace/main /usr/bin/virtink-ccm
ENTRYPOINT ["virtink-ccm"]
