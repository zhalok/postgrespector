FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o pg-buffer-usage-exporter .

FROM alpine:3.20

COPY --from=build /app/pg-buffer-usage-exporter /usr/local/bin/pg-buffer-usage-exporter

EXPOSE 9108

CMD ["pg-buffer-usage-exporter"]
