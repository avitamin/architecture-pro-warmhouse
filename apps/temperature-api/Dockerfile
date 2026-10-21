FROM golang:1.25.4-alpine3.22 AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /temperature-api .

FROM alpine:3.22
RUN addgroup -S temperature && adduser -S -G temperature temperature
COPY --from=builder /temperature-api /usr/local/bin/temperature-api
USER temperature
EXPOSE 8081
CMD ["temperature-api"]
