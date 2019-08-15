# https://www.cloudreach.com/en/insights/blog/containerize-this-how-to-build-golang-dockerfiles/
FROM golang:alpine as builder
RUN mkdir /build
ADD . /build/
WORKDIR /build

# need git to get dependencies
RUN apk update && apk upgrade && apk add --no-cache bash git openssh
RUN go get github.com/gorilla/websocket

# building single binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o main .

# 0KB container
FROM scratch
COPY --from=builder /build/main/ /app/
WORKDIR /app
CMD ["./main"]
EXPOSE 8080
