FROM --platform=$BUILDPLATFORM golang:1.26 AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION

WORKDIR /src

# Download dependencies first so they are cached across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /bin/argocd-watcher .

FROM gcr.io/distroless/static:nonroot

COPY --from=build /bin/argocd-watcher /usr/local/bin/argocd-watcher

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/argocd-watcher"]
