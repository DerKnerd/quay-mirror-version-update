FROM golang:1.16-alpine
WORKDIR /app
COPY . .

RUN go build -o /qauy-mirror-version-update

CMD ["/qauy-mirror-version-update"]