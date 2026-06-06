FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /syncviva .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /syncviva /usr/local/bin/syncviva
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["syncviva"]
