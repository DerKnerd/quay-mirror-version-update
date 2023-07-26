FROM quay.imanuel.dev/dockerhub/library---golang:1.20-alpine as build
WORKDIR /app
COPY . .

RUN apk update
RUN apk add git
RUN go build -o /quay-mirror-version-update .

FROM quay.imanuel.dev/dockerhub/library---alpine:latest

COPY --from=build /quay-mirror-version-update /quay-mirror-version-update

CMD ["/quay-mirror-version-update"]