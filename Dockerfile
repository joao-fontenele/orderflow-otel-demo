FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/orders ./cmd/orders
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/inventory ./cmd/inventory
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/email ./cmd/email
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/migrate ./cmd/migrate

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /bin/gateway /bin/gateway
COPY --from=builder /bin/orders /bin/orders
COPY --from=builder /bin/inventory /bin/inventory
COPY --from=builder /bin/worker /bin/worker
COPY --from=builder /bin/email /bin/email
COPY --from=builder /bin/migrate /bin/migrate

COPY migrations ./migrations

ARG SERVICE
ENV SERVICE=${SERVICE}

ENTRYPOINT ["/bin/sh", "-c", "/bin/${SERVICE} \"$@\"", "--"]
