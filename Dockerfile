FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /parsergo ./cmd/parsergo

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /parsergo /usr/local/bin/parsergo
EXPOSE 3120
ENTRYPOINT ["parsergo", "serve"]
