FROM golang:1.24-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY main.go ./
RUN go build -o orders-reader .

FROM alpine:3.20

COPY --from=build /app/orders-reader /usr/local/bin/orders-reader

CMD ["orders-reader"]
