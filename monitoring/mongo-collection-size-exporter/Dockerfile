FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o mongo-collection-size-exporter .

FROM alpine:3.20

COPY --from=build /app/mongo-collection-size-exporter /usr/local/bin/mongo-collection-size-exporter

EXPOSE 9103

CMD ["mongo-collection-size-exporter"]
