# syntax=docker/dockerfile:1
FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download && go mod verify
COPY backend/ ./
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63
RUN apk add --no-cache ca-certificates
COPY --from=build /out/migrate /app/migrate
USER 10001:10001
ENTRYPOINT ["/app/migrate"]
