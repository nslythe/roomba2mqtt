FROM golang:1.20 as build

COPY . /app

WORKDIR /app

RUN go mod download && go mod verify
RUN go build .

CMD ["/app/roomba2mqtt"]