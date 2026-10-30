FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/plata ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/plata /bin/plata
EXPOSE 8080
ENTRYPOINT ["/bin/plata"]