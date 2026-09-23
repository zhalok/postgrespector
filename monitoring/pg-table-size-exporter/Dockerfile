FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o pg-table-size-exporter .

FROM alpine:3.20

COPY --from=build /app/pg-table-size-exporter /usr/local/bin/pg-table-size-exporter

EXPOSE 9102

CMD ["pg-table-size-exporter"]
