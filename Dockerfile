FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /outage-radar ./cmd/outage-radar

FROM gcr.io/distroless/static
COPY --from=build /outage-radar /outage-radar
ENTRYPOINT ["/outage-radar"]
