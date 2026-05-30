FROM golang:1.26-trixie AS build

RUN mkdir /app
COPY . /app
WORKDIR /app

RUN go tool task build

FROM alpine:3

COPY --from=build /app/musikat /

ENTRYPOINT ["/musikat"]
