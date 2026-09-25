# Build natively and cross-compile: no QEMU emulation on an ARM Mac.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# distroless/static has CA certificates for HTTPS; zones are embedded via time/tzdata.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bot /bot
ENV CONFIG_PATH=/data/config.yaml \
    DB_PATH=/data/organizer.db \
    GOOGLE_SA_FILE=/data/sa.json
VOLUME /data
ENTRYPOINT ["/bot"]
