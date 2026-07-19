# Multi-stage build for the mogi-access service. A static binary on distroless
# keeps the image tiny and dependency-free (the service is pure Go — no cgo),
# and the deploy deterministic regardless of buildpack Go-version support.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO off → a fully static binary that runs on the distroless static image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/mogi-access .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mogi-access /mogi-access
# Cloud Run injects PORT (usually 8080); the service reads it (default 8787).
ENTRYPOINT ["/mogi-access"]
