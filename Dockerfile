FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /laia ./cmd/laia

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
RUN mkdir -p /data
COPY --from=build /laia /laia
ENV DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/laia"]
