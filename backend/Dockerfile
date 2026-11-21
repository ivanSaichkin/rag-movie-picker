# syntax=docker/dockerfile:1

# The module uses a local replace for the gRPC contract, so the build context
# is the repository root and both modules are copied.
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY pkg/ragapi ./pkg/ragapi
COPY backend/go.mod backend/go.sum ./backend/

WORKDIR /src/backend
RUN go mod download

COPY backend/ ./

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app

COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate

USER app
EXPOSE 8080

ENTRYPOINT ["/app/api"]
