FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download && go mod verify
COPY backend/ ./
RUN go tool templ generate
ENV GOCACHE=/tmp/go-build
USER 10001:10001
ENTRYPOINT ["go", "test"]
CMD ["-race", "-count=1", "./..."]
