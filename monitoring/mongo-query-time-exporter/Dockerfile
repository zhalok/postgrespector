FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o mongo-query-time-exporter .

FROM alpine:3.20

COPY --from=build /app/mongo-query-time-exporter /usr/local/bin/mongo-query-time-exporter

EXPOSE 9108

CMD ["mongo-query-time-exporter"]
