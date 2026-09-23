FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN go build -o mongo-total-size-exporter .

FROM alpine:3.20

COPY --from=build /app/mongo-total-size-exporter /usr/local/bin/mongo-total-size-exporter

EXPOSE 9106

CMD ["mongo-total-size-exporter"]
