FROM golang:1.24-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY main.go ./
RUN go build -o cgroup-exporter .

FROM alpine:3.20

COPY --from=build /app/cgroup-exporter /usr/local/bin/cgroup-exporter

EXPOSE 9100

CMD ["cgroup-exporter"]
