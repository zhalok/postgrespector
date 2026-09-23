FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o postgres-hit-read-stats-exporter .

FROM alpine:3.20

COPY --from=build /app/postgres-hit-read-stats-exporter /usr/local/bin/postgres-hit-read-stats-exporter

EXPOSE 9101

CMD ["postgres-hit-read-stats-exporter"]
