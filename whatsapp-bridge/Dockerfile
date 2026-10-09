FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=1 go build -o /out/wa-bridge .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
ENV TZ=America/El_Salvador WHATSAPP_BRIDGE_BIND=0.0.0.0 WHATSAPP_BRIDGE_PORT=8590
WORKDIR /data
COPY --from=build /out/wa-bridge /usr/local/bin/wa-bridge
VOLUME ["/data"]
EXPOSE 8590
CMD ["wa-bridge"]
