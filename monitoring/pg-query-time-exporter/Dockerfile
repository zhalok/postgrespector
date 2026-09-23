FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o pg-query-time-exporter .

FROM alpine:3.20

COPY --from=build /app/pg-query-time-exporter /usr/local/bin/pg-query-time-exporter

EXPOSE 9107

CMD ["pg-query-time-exporter"]
