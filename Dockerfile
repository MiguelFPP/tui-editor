# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine AS base
WORKDIR /src
ENV CGO_ENABLED=0
ENV GOTOOLCHAIN=local
# go.sum does not exist yet on the first build, so this glob is a no-op until `make tidy` generates it.
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .

FROM base AS dev
# git and openssh-client back the Git adapter; gcc and musl-dev let `make test-race` build the race detector.
RUN apk add --no-cache git openssh-client gcc musl-dev
# The bind-mounted /src is owned by the host user; allow git to read its metadata.
RUN git config --system --add safe.directory /src
RUN --mount=type=cache,target=/go/pkg/mod go install github.com/air-verse/air@v1.67.4
CMD ["air", "-c", ".air.toml"]

FROM base AS build
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/tui-editor ./cmd/tui-editor

FROM alpine:3.24.4 AS runtime
# ncurses-terminfo-base ships the terminfo database Bubble Tea needs to render inside the container.
RUN apk add --no-cache ca-certificates tzdata ncurses-terminfo-base git openssh-client \
    && adduser -D -H -u 10001 app
COPY --from=build /out/tui-editor /usr/local/bin/tui-editor
USER app
WORKDIR /home/app
ENTRYPOINT ["/usr/local/bin/tui-editor"]
