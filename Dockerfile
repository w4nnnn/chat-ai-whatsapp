FROM golang:1.26-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/bot ./cmd/bot

FROM alpine:3.21 AS production

RUN apk add --no-cache ca-certificates

COPY --from=build /app/bot /usr/local/bin/bot

VOLUME ["/app/data"]
EXPOSE 3000

CMD ["bot"]
