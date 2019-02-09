FROM golang:1.11-alpine

ADD . /valkyrie
WORKDIR /valkyrie

RUN apk add --no-cache git gcc musl-dev
RUN apk add --no-cache lame lame-dev
RUN go get -d -u ./...
RUN go build github.com/R-a-dio/valkyrie/cmd/hanyuu
ENTRYPOINT ["/bin/ash"]