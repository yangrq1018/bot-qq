FROM golang:1.18.1-alpine as builder

ARG http_proxy
ARG https_proxy
RUN apk add --no-cache make git build-base

WORKDIR /src

# copy dependency file first, avoid frequent go.mod download
COPY go.mod /src
RUN go mod download

# copy source files
COPY . /src
RUN go build -o robot . && mv ./robot /robot

FROM alpine:latest

# for time.LoadLocation
RUN apk add --no-cache ca-certificates tzdata libc6-compat libgcc libstdc++
# the trailing slash is a must for .json to get copied to directory /etc/jerry/
COPY --from=builder /robot /src/device.json /

ENTRYPOINT ["/robot"]
