FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o pg-total-relation-size-exporter .

FROM alpine:3.20

COPY --from=build /app/pg-total-relation-size-exporter /usr/local/bin/pg-total-relation-size-exporter

EXPOSE 9105

CMD ["pg-total-relation-size-exporter"]
