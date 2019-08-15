FROM golang:alpine as builder
RUN mkdir /build
ADD . /build/
WORKDIR /build
# need git to get dependencies
RUN apk update && apk upgrade && apk add --no-cache bash git openssh
RUN go get github.com/gorilla/websocket
RUN go build -o main .

FROM alpine
COPY --from=builder /build/main/ /app/
WORKDIR /app
CMD ["./main"]
EXPOSE 8080
