FROM golang:1.18-alpine as builder
WORKDIR /build
COPY . .

RUN apk add git
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w"

# generate clean, final image for end users
FROM alpine:latest
COPY --from=builder /build/light .

EXPOSE 80
EXPOSE 8443

ENTRYPOINT [ "./light" ]
CMD [ "server", "--address", "0.0.0.0" ]