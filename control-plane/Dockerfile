# Build the control-plane binary and ship it on a minimal base.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/aevora-controld ./cmd/aevora-controld

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aevora-controld /aevora-controld
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/aevora-controld"]
