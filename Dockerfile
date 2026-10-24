# Build natively and cross-compile: no QEMU emulation on an ARM Mac.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The commit and the build time from make image, after the module download so that a new
# commit does not invalidate its cached layer.
ARG VERSION=dev BUILT=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION -X main.built=$BUILT" -o /out/bot ./cmd/bot

# distroless/static has CA certificates for HTTPS; zones are embedded via time/tzdata.
FROM gcr.io/distroless/static-debian12:nonroot
# An ARG ends with its stage: the labels need it declared again. make version reads them.
ARG VERSION=dev BUILT=unknown
LABEL org.opencontainers.image.revision=$VERSION org.opencontainers.image.created=$BUILT
COPY --from=build /out/bot /bot
ENV CONFIG_PATH=/data/config.yaml \
    DB_PATH=/data/organizer.db \
    GOOGLE_SA_FILE=/data/sa.json
VOLUME /data
ENTRYPOINT ["/bot"]
