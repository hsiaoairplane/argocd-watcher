FROM --platform=$BUILDPLATFORM golang:1.26 AS build

ARG BUILDPLATFORM
ARG TARGETARCH
ARG VERSION

COPY . .
RUN GOOS=linux GOARCH=$TARGETARCH go build -o /bin/argocd-watcher .

FROM golang:1.26

COPY --from=build /bin/argocd-watcher /usr/local/bin/argocd-watcher

ENTRYPOINT ["/usr/local/bin/argocd-watcher"]
