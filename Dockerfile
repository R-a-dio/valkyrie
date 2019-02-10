FROM golang:1.11-alpine AS build-env

RUN apk add --no-cache git gcc musl-dev
RUN apk add --no-cache lame lame-dev

COPY . /valkyrie
WORKDIR /valkyrie

RUN go get -d -u ./...
RUN go build github.com/R-a-dio/valkyrie/cmd/hanyuu

# create a new image without the compilers and dependencies
FROM alpine

# configuration volume, can also be mounted directly into the WORKDIR
VOLUME ["/config"]
ENV HANYUU_CONFIG=/config/hanyuu.toml

# runtime dependencies
RUN apk add --no-cache lame-dev

WORKDIR /valkyrie
# add a group and user
RUN addgroup -S valkyrie && adduser -S valkyrie -G valkyrie
# switch to it
USER valkyrie:valkyrie
# copy our build artifact from the previous stage
COPY --from=build-env /valkyrie/hanyuu /valkyrie
# update PATH so it's on there
ENV PATH /valkyrie:$PATH

ENTRYPOINT ["/bin/ash"]