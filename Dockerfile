FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY hc.go ./
COPY cmd/hc-demo ./cmd/hc-demo

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/hc-demo ./cmd/hc-demo

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/hc-demo /hc-demo

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/hc-demo"]
