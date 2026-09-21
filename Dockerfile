FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/consumer ./cmd/consumer \
 && CGO_ENABLED=0 go build -o /app/loadgen ./cmd/loadgen \
 && CGO_ENABLED=0 go build -o /app/summarize ./cmd/summarize

FROM alpine:3.22
COPY --from=build /app/ /app/
