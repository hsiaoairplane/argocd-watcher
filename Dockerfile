FROM --platform=$BUILDPLATFORM golang:1.24 AS build

ARG BUILDPLATFORM
ARG TARGETARCH
ARG VERSION

COPY . .
RUN GOOS=linux GOARCH=$TARGETARCH go build -o /bin/argocd-watcher .

FROM golang:1.24

COPY --from=build /bin/argocd-watcher /usr/local/bin/argocd-watcher

ENTRYPOINT ["/usr/local/bin/argocd-watcher"]
