FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o mongo-hit-read-stats-exporter .

FROM alpine:3.20

COPY --from=build /app/mongo-hit-read-stats-exporter /usr/local/bin/mongo-hit-read-stats-exporter

EXPOSE 9104

CMD ["mongo-hit-read-stats-exporter"]
