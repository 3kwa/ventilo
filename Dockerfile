FROM golang:alpine
RUN mkdir /app
ADD . /app/
WORKDIR /app
# need git to get dependencies
RUN apk update && apk upgrade && apk add --no-cache bash git openssh
RUN go get github.com/gorilla/websocket
RUN go build -o main .
CMD ["./main"]
EXPOSE 8080
