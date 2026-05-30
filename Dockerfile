FROM golang:1.26-trixie AS build

RUN mkdir /app
COPY . /app
WORKDIR /app

RUN go tool task build

FROM alpine:3

COPY --from=build /app/musikat /

RUN mkdir /data

ENV MUSIKAT_DB_PATH=/data/musikat.db
ENV MUSIKAT_SERVER_LISTEN_ADDR=:8080

EXPOSE 8080

ENTRYPOINT ["/musikat"]
