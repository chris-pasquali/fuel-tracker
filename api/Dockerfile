FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fillwise ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/fillwise /app/fillwise
EXPOSE 8080
ENTRYPOINT ["/app/fillwise"]
