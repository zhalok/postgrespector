FROM golang:1.24-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY main.go ./
RUN go build -o orders-client .

FROM alpine:3.20

COPY --from=build /app/orders-client /usr/local/bin/orders-client

CMD ["orders-client"]
