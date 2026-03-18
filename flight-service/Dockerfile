FROM golang:1.23 AS builder
WORKDIR /app

# Install protoc via apt (uses HTTP, no SSL issues)
RUN apt-get update -qq && \
    apt-get install -y --no-install-recommends \
    protobuf-compiler \
    libprotobuf-dev && \
    rm -rf /var/lib/apt/lists/*

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.2 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1


ENV PATH="$PATH:$(go env GOPATH)/bin"

COPY . .

# Генерируем Go-код из proto (настоящие proto.Message структуры)
RUN mkdir -p gen && \
    protoc \
      --go_out=gen --go_opt=paths=source_relative \
      --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
      -I proto -I /usr/include \
      proto/flight/v1/flight.proto

# go mod tidy видит все исходники (включая сгенерированный gen/) и скачивает зависимости
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server .
COPY migrations /migrations
EXPOSE 50051
CMD ["./server"]

