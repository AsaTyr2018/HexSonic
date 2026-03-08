FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/hexsonic-api ./cmd/hexsonic-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/hexsonic-worker ./cmd/hexsonic-worker

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/hexsonic-api /usr/local/bin/hexsonic-api
COPY --from=build /out/hexsonic-worker /usr/local/bin/hexsonic-worker
COPY web ./web
EXPOSE 8080
CMD ["hexsonic-api"]
