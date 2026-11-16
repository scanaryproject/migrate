FROM golang:1.26-alpine AS builder
ARG VERSION=dev

RUN apk add --no-cache make

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN make build VERSION=${VERSION}

FROM alpine:3.24

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates

COPY --from=builder /src/migrate /usr/local/bin/migrate
COPY cmd/migrate/migrate-up-all /usr/local/bin/migrate-up-all
RUN ln -s /usr/local/bin/migrate /migrate

ENTRYPOINT ["migrate"]
CMD ["--help"]
