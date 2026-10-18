# Go app image for the Cloudflare Container (ADR-001).
# Static binary on distroless keeps the image small, which keeps cold start short.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETARCH
WORKDIR /src
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ ./
# BUILD_SHA is written by the deploy workflow; the binary names its own
# commit (ui.DeploySHA), so pages an old container renders during a rollout
# are not stored at the edge as current. Local builds leave it empty.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X halid.org/engine/internal/ui.buildSHA=$(cat BUILD_SHA 2>/dev/null | tr -cd 0-9a-f)" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
